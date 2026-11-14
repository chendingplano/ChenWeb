// Command compliance-gold builds the Compliance Review gold set (openspec compliance-reviewer-phase2).
//
// The first iteration's gold is built by the coding agent, following the compliance-review-benchmark
// skill (.agents/skills/compliance-review-benchmark), in these steps:
//
//	compliance-gold chunks -record 416 [-out chunks.txt]
//	compliance-gold pool   -snapshot 3 -record 416 -activities acts.json -out pool.json [-user <user id>]
//	compliance-gold save   -pool pool.json -labels labels.jsonl -model-name <model id> \
//	                       -rules-dir <skill dir>/rules -rules-version 1.0.0 [-audit-notes audit.md]
//	compliance-gold show   -run 20261114_150000
//
// panel runs the deferred model-panel labelling: two model annotators and a model adjudicator named
// by COMPLIANCE_REVIEWER_ANNOTATOR_1, COMPLIANCE_REVIEWER_ANNOTATOR_2 and COMPLIANCE_ADJUDICATOR.
//
//	compliance-gold panel -snapshot 3 -user <user id> [-check]
//
// A document must be listed in [compliance-review.covered-docs] or [compliance-review.uncovered-docs]
// of doc-review.local.toml, and [reviewers.compliance].max_candidates caps each activity's pool. The
// pool searches the whole knowledge base and leaves out the document, its byte-identical copies and
// the records indexed after the snapshot. Run from ChenWeb with `mise exec --`.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	complianceeval "github.com/chendingplano/deepdoc/server/api/compliance-eval"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	docreviews "github.com/chendingplano/deepdoc/server/api/doc-reviews"
	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

const usage = `usage: compliance-gold chunks -record N [-out FILE]
       compliance-gold pool -snapshot N -record N -activities FILE -out FILE [-user ID] [-config FILE]
       compliance-gold save -pool FILE -labels FILE -model-name ID -rules-dir DIR -rules-version V [-audit-notes FILE] [-config FILE]
       compliance-gold show -run KEY
       compliance-gold panel -snapshot N -user ID [-check] [-config FILE]`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	cmds := map[string]func([]string) error{"chunks": cmdChunks, "pool": cmdPool, "save": cmdSave, "show": cmdShow, "panel": cmdPanel}
	cmd, ok := cmds[os.Args[1]]
	if !ok {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err := cmd(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// usageError makes main exit with status 2 after printing the usage.
func usageError(msg string) error {
	fmt.Fprintln(os.Stderr, msg)
	fmt.Fprintln(os.Stderr, usage)
	os.Exit(2)
	return nil
}

func loadConfig(path string) (*complianceeval.GoldConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return complianceeval.ParseGoldConfig(raw)
}

func listedDoc(cfg *complianceeval.GoldConfig, cfgPath string, record int64) (complianceeval.GoldDoc, error) {
	d, ok := cfg.Find(record)
	if !ok {
		return d, fmt.Errorf("record %d is not listed in [compliance-review.covered-docs] or [compliance-review.uncovered-docs] of %s", record, cfgPath)
	}
	return d, nil
}

// verifySnapshot refuses a snapshot whose records changed or disappeared, and returns the records
// indexed since it was recorded.
func verifySnapshot(ctx context.Context, db *sql.DB, snapshot int64) ([]int64, error) {
	diffs, added, err := complianceeval.Verify(ctx, db, snapshot, os.Getenv("EMBEDDING_MODEL_NAME"))
	if err != nil {
		return nil, err
	}
	if len(diffs) > 0 {
		for _, d := range diffs {
			fmt.Fprintln(os.Stderr, "snapshot drift:", d)
		}
		return nil, fmt.Errorf("snapshot %d no longer matches the knowledge base; record a new snapshot", snapshot)
	}
	return added, nil
}

func searchFunc(db *sql.DB) func(context.Context, searchengine.Request) (*searchengine.Response, error) {
	cfg := searchengine.LoadConfig()
	return func(ctx context.Context, req searchengine.Request) (*searchengine.Response, error) {
		return searchengine.Search(ctx, db, cfg, req)
	}
}

func cmdChunks(args []string) error {
	fs := flag.NewFlagSet("chunks", flag.ExitOnError)
	record := fs.Int64("record", 0, "kb.inputs record id")
	out := fs.String("out", "", "file to write (default stdout)")
	fs.Parse(args)
	if *record <= 0 {
		return usageError("chunks needs -record")
	}
	ctx := context.Background()
	db, err := complianceeval.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()
	chunks, err := docreviews.LoadGoldChunks(ctx, db, *record)
	if err != nil {
		return err
	}
	w := os.Stdout
	if *out != "" {
		f, err := os.Create(*out)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	fmt.Fprintf(w, "# record %d: %d chunks, the lines the Compliance Review reviewer reads (line number, TAB, content)\n", *record, len(chunks))
	if dc := docreviews.GoldDocContext(chunks[0].InputJSON); dc != "" {
		fmt.Fprintf(w, "# doc_context: %s\n", dc)
	}
	for _, ch := range chunks {
		fmt.Fprintf(w, "\n## chunk %d: lines %d-%d\n", ch.Index, ch.StartLine, ch.EndLine)
		for _, l := range docreviews.GoldChunkLines(ch.InputJSON) {
			fmt.Fprintf(w, "%d\t%s\n", l.Number, strings.ReplaceAll(l.Content, "\n", " "))
		}
	}
	if *out != "" {
		fmt.Printf("wrote %d chunks of record %d to %s\n", len(chunks), *record, *out)
	}
	return nil
}

func cmdPool(args []string) error {
	fs := flag.NewFlagSet("pool", flag.ExitOnError)
	snapshot := fs.Int64("snapshot", 0, "evaluation snapshot id (see compliance-snapshot)")
	record := fs.Int64("record", 0, "kb.inputs record id of the document")
	actsPath := fs.String("activities", "", "the agent's activities file (JSON)")
	out := fs.String("out", "", "pool file to write (JSON)")
	cfgPath := fs.String("config", "doc-review.local.toml", "config file")
	user := fs.String("user", "", "user id the search's model calls are attributed to (default: the document's owner)")
	fs.Parse(args)
	if *snapshot <= 0 || *record <= 0 || *actsPath == "" || *out == "" {
		return usageError("pool needs -snapshot, -record, -activities and -out")
	}
	ctx := context.Background()
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	if _, err := listedDoc(cfg, *cfgPath, *record); err != nil {
		return err
	}
	raw, err := os.ReadFile(*actsPath)
	if err != nil {
		return err
	}
	var af complianceeval.ActivitiesFile
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&af); err != nil {
		return fmt.Errorf("activities file %s: %w", *actsPath, err)
	}
	if af.RecordID != *record {
		return fmt.Errorf("activities file %s is for record %d, not %d", *actsPath, af.RecordID, *record)
	}
	db, err := complianceeval.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()
	// The pool searches call an embedding model and the keyword model; attribute their usage.
	if *user == "" {
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(user_id::text, '') FROM kb.inputs WHERE id = $1`, *record).Scan(user); err != nil {
			return fmt.Errorf("read owner of record %d: %w", *record, err)
		}
		if *user == "" {
			return fmt.Errorf("record %d has no owner; pass -user", *record)
		}
	}
	ctx = docprocessing.WithLLMUserID(ctx, *user)
	added, err := verifySnapshot(ctx, db, *snapshot)
	if err != nil {
		return err
	}
	chunks, err := docreviews.LoadGoldChunks(ctx, db, *record)
	if err != nil {
		return err
	}
	acts, err := complianceeval.CheckActivities(chunks, af.Activities)
	if err != nil {
		return err
	}
	copies, err := docreviews.SameMD5RecordIDs(ctx, db, *record)
	if err != nil {
		return err
	}
	exclude := append(append([]int64(nil), copies...), added...)
	p := &complianceeval.Pipeline{MaxCandidates: cfg.MaxCandidates, Search: searchFunc(db)}
	pas, err := complianceeval.PoolActivities(ctx, p, acts, exclude)
	if err != nil {
		return err
	}
	pf := complianceeval.PoolFile{SnapshotID: *snapshot, RecordID: *record, MaxCandidates: cfg.MaxCandidates,
		ExcludedRecords: copies, AddedSinceSnapshot: len(added), Activities: pas}
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, append(data, '\n'), 0o644); err != nil {
		return err
	}
	total := 0
	for _, pa := range pas {
		total += len(pa.Candidates)
	}
	fmt.Printf("wrote %s: %d activities, %d candidates to label (at most %d per activity), snapshot %d; %d record(s) indexed after the snapshot left out; search model usage attributed to user %s\n",
		*out, len(pas), total, cfg.MaxCandidates, *snapshot, len(added), *user)
	return nil
}

func cmdSave(args []string) error {
	fs := flag.NewFlagSet("save", flag.ExitOnError)
	poolPath := fs.String("pool", "", "pool file written by compliance-gold pool")
	labelsPath := fs.String("labels", "", "the agent's labels file (one JSON object per line)")
	model := fs.String("model-name", "", "model id of the agent that labelled")
	rulesDir := fs.String("rules-dir", "", "the skill's rules directory (holds versions.json)")
	rulesVersion := fs.String("rules-version", "", "rules version followed, e.g. 1.0.0")
	auditPath := fs.String("audit-notes", "", "optional file with the agent's audit notes")
	cfgPath := fs.String("config", "doc-review.local.toml", "config file")
	fs.Parse(args)
	if *poolPath == "" || *labelsPath == "" || strings.TrimSpace(*model) == "" || *rulesDir == "" || *rulesVersion == "" {
		return usageError("save needs -pool, -labels, -model-name, -rules-dir and -rules-version")
	}
	ctx := context.Background()
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(*poolPath)
	if err != nil {
		return err
	}
	var pf complianceeval.PoolFile
	if err := json.Unmarshal(raw, &pf); err != nil {
		return fmt.Errorf("pool file %s: %w", *poolPath, err)
	}
	doc, err := listedDoc(cfg, *cfgPath, pf.RecordID)
	if err != nil {
		return err
	}
	rules, err := complianceeval.LoadRulesVersion(*rulesDir, *rulesVersion)
	if err != nil {
		return err
	}
	lf, err := os.Open(*labelsPath)
	if err != nil {
		return err
	}
	labels, err := complianceeval.ParseLabels(lf)
	lf.Close()
	if err != nil {
		return err
	}
	audit := ""
	if *auditPath != "" {
		b, err := os.ReadFile(*auditPath)
		if err != nil {
			return err
		}
		audit = strings.TrimSpace(string(b))
	}
	db, err := complianceeval.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := verifySnapshot(ctx, db, pf.SnapshotID); err != nil {
		return err
	}
	chunks, err := docreviews.LoadGoldChunks(ctx, db, pf.RecordID)
	if err != nil {
		return err
	}
	in := make([]complianceeval.AgentActivity, len(pf.Activities))
	for i, pa := range pf.Activities {
		in[i] = pa.AgentActivity
	}
	acts, err := complianceeval.CheckActivities(chunks, in)
	if err != nil {
		return fmt.Errorf("pool file %s: %w", *poolPath, err)
	}
	gd, err := complianceeval.BuildAgentDocument(pf, acts, labels, doc)
	if err != nil {
		return err
	}
	run := &complianceeval.GoldRun{
		Method: complianceeval.MethodAgent, SnapshotID: pf.SnapshotID, RulesFile: rules.File, RulesVersion: rules.Version,
		RulesSHA256: rules.SHA256, Profiles: complianceeval.Profiles{Annotator1: strings.TrimSpace(*model)},
		MaxCandidates: pf.MaxCandidates, AuditNotes: audit, Documents: []*complianceeval.GoldDocument{gd},
	}
	key, id, err := complianceeval.SaveRun(ctx, db, run, time.Now())
	if err != nil {
		return err
	}
	fmt.Printf("saved gold run %s (id %d): record %d, %d activities, %d labels; labelled by %s alone (agent), rules %s\n",
		key, id, pf.RecordID, len(gd.Activities), len(labels), run.Profiles.Annotator1, rules.File)
	return nil
}

func cmdShow(args []string) error {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	key := fs.String("run", "", "gold run key")
	fs.Parse(args)
	if *key == "" {
		return usageError("show needs -run")
	}
	db, err := complianceeval.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()
	s, err := complianceeval.LoadRunSummary(context.Background(), db, *key)
	if err != nil {
		return err
	}
	fmt.Printf("gold run %s (id %d): method %s, labeller %s, snapshot %d, rules %s (%s), max %d candidates per activity\n",
		s.Key, s.ID, s.Method, s.Annotator1, s.SnapshotID, s.RulesFile, s.RulesVersion, s.MaxCandidates)
	fmt.Printf("  records %v: %d activities (%d document-level), %d with a governing candidate\n",
		s.Records, s.Activities, s.DocumentLevel, s.ActivitiesWithGoverns)
	fmt.Printf("  labels: governs %d, topical %d, unrelated %d, ambiguous %d\n",
		s.Labels["governs"], s.Labels["topical"], s.Labels["unrelated"], s.Labels["ambiguous"])
	return nil
}

func cmdPanel(args []string) error {
	fs := flag.NewFlagSet("panel", flag.ExitOnError)
	snapshot := fs.Int64("snapshot", 0, "evaluation snapshot id (see compliance-snapshot)")
	cfgPath := fs.String("config", "doc-review.local.toml", "config file with the [compliance-review.*] tables")
	user := fs.String("user", "", "user id the LLM calls are attributed to")
	check := fs.Bool("check", false, "validate everything but call no model and write nothing")
	fs.Parse(args)
	if *snapshot <= 0 || (*user == "" && !*check) {
		return usageError("panel needs -snapshot and -user (or -check)")
	}
	ctx := context.Background()
	if *user != "" {
		ctx = docprocessing.WithLLMUserID(ctx, *user)
	}
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	docs, _ := cfg.Documents()
	if len(docs) == 0 {
		return fmt.Errorf("no documents listed in [compliance-review.covered-docs] or [compliance-review.uncovered-docs] of %s", *cfgPath)
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
	added, err := verifySnapshot(ctx, db, *snapshot)
	if err != nil {
		return err
	}
	if *check {
		fmt.Printf("ok: %d documents, snapshot %d holds (%d records added since), profiles %s / %s / %s\n", len(docs), *snapshot,
			len(added), profiles.Annotator1, profiles.Annotator2, profiles.Adjudicator)
		return nil
	}

	build := func(ref string) (complianceeval.Labeller, error) {
		c, model, err := docprocessing.BuildReviewerLLMClient(ref)
		if err != nil {
			return complianceeval.Labeller{}, fmt.Errorf("profile %q: %w", ref, err)
		}
		return complianceeval.Labeller{Name: ref, Model: model, Client: c}, nil
	}
	p := complianceeval.Pipeline{MaxCandidates: cfg.MaxCandidates, Search: searchFunc(db)}
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
	p.Chunks = func(ctx context.Context, id int64) ([]docreviews.GoldChunk, error) {
		return docreviews.LoadGoldChunks(ctx, db, id)
	}
	p.Exclude = func(ctx context.Context, id int64) ([]int64, error) {
		copies, err := docreviews.SameMD5RecordIDs(ctx, db, id)
		return append(copies, added...), err
	}

	run := &complianceeval.GoldRun{
		Method: complianceeval.MethodModelPanel, SnapshotID: *snapshot, RulesFile: complianceeval.RulesFile,
		RulesVersion: "v1", RulesSHA256: p.Prompts.RulesSHA256, Profiles: profiles, MaxCandidates: cfg.MaxCandidates,
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
