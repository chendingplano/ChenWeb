package categorytree

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// Model is the decision model a run uses, with what the classification record names.
type Model struct {
	Decider    Decider
	ModelName  string
	Profile    string
	PromptRefs map[string]string
}

// Service runs classifications: it loads the artifact type's tree and decision model, walks
// each artifact and stores the outcome.
type Service struct {
	Store   *Store
	Sources map[string]ArtifactSource
	Model   func() (*Model, error)
	Options Options
	Logger  ApiTypes.JimoLogger
	// ArtifactConcurrency bounds artifacts classified at once (default 4).
	ArtifactConcurrency int
}

// NewService wires the production store, the metrics source and the env-configured model.
func NewService(store *Store, logger ApiTypes.JimoLogger) *Service {
	return &Service{
		Store:   store,
		Sources: map[string]ArtifactSource{"metrics": MetricsSource{DB: store.DB}},
		Model: func() (*Model, error) {
			d, err := NewJevDeciderFromEnv(logger)
			if err != nil {
				return nil, err
			}
			return &Model{Decider: d, ModelName: d.ModelName, Profile: d.Profile, PromptRefs: d.PromptRefs()}, nil
		},
		Options: OptionsFromEnv(),
		Logger:  logger,
	}
}

// OptionsFromEnv reads CATEGORY_BATCH_SIZE, CATEGORY_MAX_REQUESTS, CATEGORY_MATCH_MIN_P and
// CATEGORY_NOMATCH_MAX_P; unset or invalid values take the defaults.
func OptionsFromEnv() Options {
	atoi := func(k string) int { n, _ := strconv.Atoi(strings.TrimSpace(os.Getenv(k))); return n }
	atof := func(k string) float64 { f, _ := strconv.ParseFloat(strings.TrimSpace(os.Getenv(k)), 64); return f }
	return Options{
		BatchSize:   atoi("CATEGORY_BATCH_SIZE"),
		MaxRequests: atoi("CATEGORY_MAX_REQUESTS"),
		MatchMinP:   atof("CATEGORY_MATCH_MIN_P"),
		NoMatchMaxP: atof("CATEGORY_NOMATCH_MAX_P"),
	}.withDefaults()
}

// RunStart describes a run that was accepted and continues in the background.
type RunStart struct {
	ArtifactType    string `json:"artifact_type"`
	TreeID          string `json:"tree_id"`
	TreeSeq         int64  `json:"tree_seq"`
	Artifacts       int    `json:"artifacts"`
	RemovedBindings int64  `json:"removed_bindings"`
}

type runPlan struct {
	src   ArtifactSource
	info  *TreeInfo
	tree  *Tree
	model *Model
}

func (s *Service) prepare(ctx context.Context, artifactType string) (*runPlan, error) {
	src, ok := s.Sources[artifactType]
	if !ok {
		return nil, inputErr("artifact type %q has no source for classification", artifactType)
	}
	info, err := s.Store.TreeForType(ctx, artifactType)
	if err != nil {
		return nil, err
	}
	tree, err := s.Store.LoadCurrent(ctx, info)
	if err != nil {
		return nil, err
	}
	if err := Validate(tree); err != nil {
		return nil, fmt.Errorf("tree %s: %w", info.CanonicalName, err) // never expected: every change set is validated
	}
	model, err := s.Model()
	if err != nil {
		return nil, &InputError{Err: err} // configuration the operator must fix; the message names it
	}
	return &runPlan{src: src, info: info, tree: tree, model: model}, nil
}

// StartRecord classifies all current artifacts of a record in the background, after removing
// the bindings of artifacts that no longer exist.
func (s *Service) StartRecord(ctx context.Context, artifactType string, recordID int64) (*RunStart, error) {
	plan, err := s.prepare(ctx, artifactType)
	if err != nil {
		return nil, err
	}
	artifacts, err := plan.src.ListRecord(ctx, recordID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(artifacts))
	for i, a := range artifacts {
		ids[i] = a.ID
	}
	removed, err := s.Store.RemoveVanished(ctx, artifactType, recordID, ids)
	if err != nil {
		return nil, err
	}
	s.Logger.Info("category classification started", "artifact_type", artifactType, "record_id", recordID,
		"artifacts", len(artifacts), "removed_bindings", removed, "tree", plan.info.CanonicalName, "tree_seq", plan.info.CurrentSeq)
	go s.classifyAll(plan, artifacts)
	return &RunStart{ArtifactType: artifactType, TreeID: plan.info.ID, TreeSeq: plan.info.CurrentSeq,
		Artifacts: len(artifacts), RemovedBindings: removed}, nil
}

// StartStale reclassifies, in the background, up to limit artifacts that have stale bindings.
func (s *Service) StartStale(ctx context.Context, artifactType string, limit int) (*RunStart, error) {
	plan, err := s.prepare(ctx, artifactType)
	if err != nil {
		return nil, err
	}
	refs, err := s.Store.StaleArtifacts(ctx, artifactType, limit)
	if err != nil {
		return nil, err
	}
	var artifacts []Artifact
	var vanished []ArtifactRef
	for _, r := range refs {
		a, err := plan.src.Get(ctx, r.ArtifactID)
		if err != nil {
			vanished = append(vanished, r)
			continue
		}
		artifacts = append(artifacts, *a)
	}
	if len(vanished) > 0 {
		s.Logger.Warn("stale bindings of missing artifacts left for record cleanup", "artifact_type", artifactType, "count", len(vanished))
	}
	go s.classifyAll(plan, artifacts)
	return &RunStart{ArtifactType: artifactType, TreeID: plan.info.ID, TreeSeq: plan.info.CurrentSeq, Artifacts: len(artifacts)}, nil
}

// StartRetry starts a new attempt for the artifact of a classification.
func (s *Service) StartRetry(ctx context.Context, classificationID int64) (*RunStart, error) {
	c, err := s.Store.GetClassification(ctx, classificationID)
	if err != nil {
		return nil, err
	}
	plan, err := s.prepare(ctx, c.ArtifactType)
	if err != nil {
		return nil, err
	}
	a, err := plan.src.Get(ctx, c.ArtifactID)
	if err != nil {
		return nil, err
	}
	go s.classifyAll(plan, []Artifact{*a})
	return &RunStart{ArtifactType: c.ArtifactType, TreeID: plan.info.ID, TreeSeq: plan.info.CurrentSeq, Artifacts: 1}, nil
}

// classifyAll runs on a fresh context: the request that started it has already returned.
func (s *Service) classifyAll(plan *runPlan, artifacts []Artifact) {
	ctx := context.Background()
	start := time.Now()
	n := s.ArtifactConcurrency
	if n <= 0 {
		n = 4
	}
	sem := make(chan struct{}, n)
	var wg sync.WaitGroup
	var mu sync.Mutex
	counts := map[string]int{}
	for _, a := range artifacts {
		wg.Add(1)
		sem <- struct{}{}
		go func(a Artifact) {
			defer wg.Done()
			defer func() { <-sem }()
			status := s.ClassifyOne(ctx, plan.info, plan.tree, plan.model, a)
			mu.Lock()
			counts[status]++
			mu.Unlock()
		}(a)
	}
	wg.Wait()
	s.Logger.Info("category classification finished", "tree", plan.info.CanonicalName, "artifacts", len(artifacts),
		"published", counts[StatusPublished], "in_review", counts[StatusInReview], "failed", counts[StatusFailed],
		"ms_used", time.Since(start).Milliseconds())
}

// ClassifyOne records an attempt, walks the tree and stores the outcome. It returns the status.
func (s *Service) ClassifyOne(ctx context.Context, info *TreeInfo, tree *Tree, model *Model, a Artifact) string {
	id, err := s.Store.StartClassification(ctx, a, info, model.ModelName, model.Profile, model.PromptRefs)
	if err != nil {
		s.Logger.Error("category classification not recorded", "artifact_type", a.Type, "artifact_id", a.ID, "error", err)
		return StatusFailed
	}
	res := Walk(ctx, tree, a, model.Decider, s.Options)
	if err := s.Store.FinishClassification(ctx, id, a, info.ID, info.CurrentSeq, res); err != nil {
		s.Logger.Error("category classification outcome not stored", "classification_id", id, "artifact_id", a.ID, "error", err)
		_ = s.Store.FailClassification(ctx, id, "store: "+err.Error())
		return StatusFailed
	}
	s.Logger.Info("category classification done", "classification_id", id, "artifact_type", a.Type, "artifact_id", a.ID,
		"record_id", a.RecordID, "status", res.Status, "reason", res.Reason, "leaves", len(res.Leaves), "requests", res.Requests)
	return res.Status
}
