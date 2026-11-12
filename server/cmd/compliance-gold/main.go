// Command compliance-gold labels the Compliance Review gold set (openspec compliance-reviewer-phase2).
//
// It reads the documents listed in doc-review.local.toml ([compliance-review.covered-docs] and
// [compliance-review.uncovered-docs]) and the organization context, checks that the evaluation
// snapshot still matches the corpus, then has two model annotators label activities and judge a
// pooled candidate list, with an adjudicator settling disagreements. The model profiles come from
// COMPLIANCE_REVIEWER_ANNOTATOR_1, COMPLIANCE_REVIEWER_ANNOTATOR_2 and COMPLIANCE_ADJUDICATOR.
//
//	cd ChenWeb && mise exec -- go run ./server/cmd/compliance-gold -snapshot 1 -user <user id> [-check]
//
// -check validates configuration, profiles and snapshot without calling any model or writing.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	complianceeval "github.com/chendingplano/deepdoc/server/api/compliance-eval"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	docreviews "github.com/chendingplano/deepdoc/server/api/doc-reviews"
	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

func main() {
	snapshot := flag.Int64("snapshot", 0, "evaluation snapshot id (see compliance-snapshot)")
	cfgPath := flag.String("config", "doc-review.local.toml", "config file with the [compliance-review.*] tables")
	poolLimit := flag.Int("pool-limit", 50, "candidates pooled per search query")
	user := flag.String("user", "", "user id the LLM calls are attributed to")
	check := flag.Bool("check", false, "validate everything but call no model and write nothing")
	flag.Parse()
	if *snapshot <= 0 || *poolLimit <= 0 || (*user == "" && !*check) {
		fmt.Fprintln(os.Stderr, "usage: compliance-gold -snapshot N -user ID [-config FILE] [-pool-limit 50] [-check]")
		os.Exit(2)
	}
	if err := run(*snapshot, *cfgPath, *poolLimit, *user, *check); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(snapshot int64, cfgPath string, poolLimit int, user string, check bool) error {
	ctx := context.Background()
	if user != "" {
		ctx = docprocessing.WithLLMUserID(ctx, user)
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	cfg, err := complianceeval.ParseGoldConfig(raw)
	if err != nil {
		return err
	}
	docs, _ := cfg.Documents()
	if len(docs) == 0 {
		return fmt.Errorf("no documents listed in [compliance-review.covered-docs] or [compliance-review.uncovered-docs] of %s", cfgPath)
	}
	profiles, err := complianceeval.ResolveProfiles(os.Getenv)
	if err != nil {
		return err
	}
	db, err := complianceeval.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	diffs, err := complianceeval.Verify(ctx, db, snapshot, os.Getenv("EMBEDDING_MODEL_NAME"))
	if err != nil {
		return err
	}
	if len(diffs) > 0 {
		for _, d := range diffs {
			fmt.Fprintln(os.Stderr, "snapshot drift:", d)
		}
		return fmt.Errorf("snapshot %d no longer matches the corpus; refusing to start", snapshot)
	}
	members, err := complianceeval.SnapshotMembers(ctx, db, snapshot)
	if err != nil {
		return err
	}
	if out := complianceeval.DocsOutsideSnapshot(docs, members); len(out) > 0 {
		return fmt.Errorf("documents %v are not members of snapshot %d", out, snapshot)
	}
	if check {
		fmt.Printf("ok: %d documents, snapshot %d holds, profiles %s / %s / %s\n", len(docs), snapshot,
			profiles.Annotator1, profiles.Annotator2, profiles.Adjudicator)
		return nil
	}

	build := func(ref string) (complianceeval.Labeller, error) {
		c, model, err := docprocessing.BuildReviewerLLMClient(ref)
		if err != nil {
			return complianceeval.Labeller{}, fmt.Errorf("profile %q: %w", ref, err)
		}
		return complianceeval.Labeller{Name: ref, Model: model, Client: c}, nil
	}
	var p complianceeval.Pipeline
	if p.A1, err = build(profiles.Annotator1); err != nil {
		return err
	}
	if p.A2, err = build(profiles.Annotator2); err != nil {
		return err
	}
	if p.Adj, err = build(profiles.Adjudicator); err != nil {
		return err
	}
	if p.Prompts, err = complianceeval.LoadPrompts(func(ref string) (string, error) {
		text, _, _, err := docprocessing.LoadPromptByRef(ref)
		return text, err
	}); err != nil {
		return err
	}
	p.PoolLimit = poolLimit
	for _, m := range members {
		p.Scope = append(p.Scope, m.RecordID)
	}
	p.Chunks = func(ctx context.Context, id int64) ([]docreviews.GoldChunk, error) {
		return docreviews.LoadGoldChunks(ctx, db, id)
	}
	p.Exclude = func(ctx context.Context, id int64) ([]int64, error) { return docreviews.SameMD5RecordIDs(ctx, db, id) }
	scfg := searchengine.LoadConfig()
	p.Search = func(ctx context.Context, req searchengine.Request) (*searchengine.Response, error) {
		return searchengine.Search(ctx, db, scfg, req)
	}

	run := &complianceeval.GoldRun{
		SnapshotID: snapshot, RulesFile: complianceeval.RulesFile, RulesSHA256: p.Prompts.RulesSHA256,
		Profiles: profiles, PoolLimit: poolLimit, Organization: cfg.OrganizationContext(),
	}
	for _, d := range docs {
		start := time.Now()
		gd, err := p.LabelDocument(ctx, d)
		if err != nil {
			return fmt.Errorf("record %d: %w (nothing saved)", d.RecordID, err)
		}
		pooled, governs := 0, 0
		for _, a := range gd.Activities {
			pooled += len(a.Judgements)
			for _, j := range a.Judgements {
				if j.Final == "governs" {
					governs++
				}
			}
		}
		fmt.Printf("record %d: %d activities (%d rejected by adjudicator), %d pooled candidates, %d governs, %s\n",
			d.RecordID, len(gd.Activities), gd.Dropped, pooled, governs, time.Since(start).Round(time.Second))
		run.Documents = append(run.Documents, gd)
	}
	key, id, err := complianceeval.SaveRun(ctx, db, run, time.Now())
	if err != nil {
		return err
	}
	fmt.Printf("saved gold run %s (id %d), LLM-adjudicated labels\n", key, id)
	return nil
}
