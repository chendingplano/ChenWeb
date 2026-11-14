// Command compliance-gold builds the Compliance Review gold set (openspec compliance-reviewer-phase2).
//
// The first iteration's gold is built by the coding agent, following the compliance-review-benchmark
// skill (.agents/skills/compliance-review-benchmark), in these steps:
//
//	compliance-gold chunks  -record 416 [-out chunks.txt]
//	compliance-gold pool    -snapshot 3 -record 416 -activities acts.json -out pool.json [-user <user id>]
//	compliance-gold expand  -pool pool.json -labels labels.jsonl -out pool.json [-user <user id>]
//	compliance-gold save    -pool pool.json -labels labels.jsonl -model-name <model id> \
//	                        -rules-dir <skill dir>/rules -rules-version 1.1.0 [-audit-notes audit.md]
//	compliance-gold show    -run 20261114_150000
//	compliance-gold augment -run 20261114_150000 -review 101 -out augment.json
//
// expand searches within every record with a governs label (repeat until it adds nothing); augment
// writes the passages a doc-review run reported that the gold run never judged, and saving their
// labels creates a new run (design D11, D12).
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
       compliance-gold expand -pool FILE -labels FILE -out FILE [-user ID] [-config FILE]
       compliance-gold save -pool FILE -labels FILE -model-name ID -rules-dir DIR -rules-version V [-audit-notes FILE] [-config FILE]
       compliance-gold show -run KEY
       compliance-gold augment -run KEY -review ID -out FILE [-config FILE]
       compliance-gold panel -snapshot N -user ID [-check] [-config FILE]`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	cmds := map[string]func([]string) error{"chunks": cmdChunks, "pool": cmdPool, "expand": cmdExpand, "save": cmdSave,
		"show": cmdShow, "augment": cmdAugment, "panel": cmdPanel}
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

// attributeSearch attributes the pool searches' model calls (embedding, keyword model) to user, or
// to the record's owner when user is empty, and returns the user id used.
func attributeSearch(ctx context.Context, db *sql.DB, record int64, user string) (context.Context, string, error) {
	if user == "" {
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(user_id::text, '') FROM kb.inputs WHERE id = $1`, record).Scan(&user); err != nil {
			return ctx, "", fmt.Errorf("read owner of record %d: %w", record, err)
		}
		if user == "" {
			return ctx, "", fmt.Errorf("record %d has no owner; pass -user", record)
		}
	}
	return docprocessing.WithLLMUserID(ctx, user), user, nil
}

// citationIndex indexes the snapshot's records a pool may search within as cited documents: the
// excluded ones (the document, its copies) left out.
func citationIndex(ctx context.Context, db *sql.DB, snapshot int64, exclude []int64) (complianceeval.CitationIndex, error) {
	members, err := complianceeval.SnapshotMembers(ctx, db, snapshot)
	if err != nil {
		return complianceeval.CitationIndex{}, err
	}
	skip := map[int64]bool{}
	for _, id := range exclude {
		skip[id] = true
	}
	var ids []int64
	for _, m := range members {
		if !skip[m.RecordID] {
			ids = append(ids, m.RecordID)
		}
	}
	names, err := complianceeval.LoadRecordNames(ctx, db, ids)
	if err != nil {
		return complianceeval.CitationIndex{}, err
	}
	return complianceeval.NewCitationIndex(names), nil
}

func readPoolFile(path string) (complianceeval.PoolFile, error) {
	var pf complianceeval.PoolFile
	raw, err := os.ReadFile(path)
	if err != nil {
		return pf, err
	}
	if err := json.Unmarshal(raw, &pf); err != nil {
		return pf, fmt.Errorf("pool file %s: %w", path, err)
	}
	return pf, nil
}

func writePoolFile(path string, pf complianceeval.PoolFile) error {
	data, err := json.MarshalIndent(pf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func readLabels(path string) ([]complianceeval.AgentLabel, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return complianceeval.ParseLabels(f)
}

// poolActivities returns the pool file's activities as the agent wrote them.
func poolActivities(pf complianceeval.PoolFile) []complianceeval.AgentActivity {
	in := make([]complianceeval.AgentActivity, len(pf.Activities))
	for i, pa := range pf.Activities {
		in[i] = pa.AgentActivity
	}
	return in
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
	ctx, who, err := attributeSearch(ctx, db, *record, *user)
	if err != nil {
		return err
	}
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
	cites, err := citationIndex(ctx, db, *snapshot, copies)
	if err != nil {
		return err
	}
	p := &complianceeval.Pipeline{MaxCandidates: cfg.MaxCandidates, Search: searchFunc(db)}
	pas, err := complianceeval.PoolActivities(ctx, p, acts, exclude, cites)
	if err != nil {
		return err
	}
	pf := complianceeval.PoolFile{SnapshotID: *snapshot, RecordID: *record, MaxCandidates: cfg.MaxCandidates,
		ExcludedRecords: copies, AddedSinceSnapshot: len(added), Activities: pas}
	if err := writePoolFile(*out, pf); err != nil {
		return err
	}
	total, cited := 0, 0
	var unresolved []string
	for _, pa := range pas {
		total += len(pa.Candidates)
		for _, c := range pa.Candidates {
			if strings.HasPrefix(c.FoundBy, "cited:") {
				cited++
			}
		}
		for _, u := range pa.UnresolvedCitedDocs {
			unresolved = append(unresolved, pa.ID+" "+u)
		}
	}
	fmt.Printf("wrote %s: %d activities, %d candidates to label (at most %d per activity from the knowledge base, plus %d from cited documents), snapshot %d; %d record(s) indexed after the snapshot left out; search model usage attributed to user %s\n",
		*out, len(pas), total, cfg.MaxCandidates, cited, *snapshot, len(added), who)
	if len(unresolved) > 0 {
		fmt.Printf("cited documents the knowledge base does not hold (report them in the audit): %s\n", strings.Join(unresolved, "; "))
	}
	return nil
}

func cmdExpand(args []string) error {
	fs := flag.NewFlagSet("expand", flag.ExitOnError)
	poolPath := fs.String("pool", "", "pool file written by compliance-gold pool (or a previous expand)")
	labelsPath := fs.String("labels", "", "the agent's labels file so far")
	out := fs.String("out", "", "pool file to write (may be the -pool file)")
	cfgPath := fs.String("config", "doc-review.local.toml", "config file")
	user := fs.String("user", "", "user id the search's model calls are attributed to (default: the document's owner)")
	fs.Parse(args)
	if *poolPath == "" || *labelsPath == "" || *out == "" {
		return usageError("expand needs -pool, -labels and -out")
	}
	ctx := context.Background()
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	pf, err := readPoolFile(*poolPath)
	if err != nil {
		return err
	}
	if _, err := listedDoc(cfg, *cfgPath, pf.RecordID); err != nil {
		return err
	}
	labels, err := readLabels(*labelsPath)
	if err != nil {
		return err
	}
	db, err := complianceeval.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, who, err := attributeSearch(ctx, db, pf.RecordID, *user)
	if err != nil {
		return err
	}
	if _, err := verifySnapshot(ctx, db, pf.SnapshotID); err != nil {
		return err
	}
	chunks, err := docreviews.LoadGoldChunks(ctx, db, pf.RecordID)
	if err != nil {
		return err
	}
	acts, err := complianceeval.CheckActivities(chunks, poolActivities(pf))
	if err != nil {
		return fmt.Errorf("pool file %s: %w", *poolPath, err)
	}
	p := &complianceeval.Pipeline{MaxCandidates: pf.MaxCandidates, Search: searchFunc(db)}
	added, err := complianceeval.ExpandPool(ctx, p, &pf, acts, labels)
	if err != nil {
		return err
	}
	if added == 0 {
		fmt.Printf("expansion complete: every record with a governs label has been searched within; %s unchanged\n", *poolPath)
		return nil
	}
	if err := writePoolFile(*out, pf); err != nil {
		return err
	}
	fmt.Printf("wrote %s: %d passages added from records with a governs label (up to %d per record); label them, then run expand again; search model usage attributed to user %s\n",
		*out, added, complianceeval.WithinPerRecord, who)
	return nil
}

func cmdAugment(args []string) error {
	fs := flag.NewFlagSet("augment", flag.ExitOnError)
	key := fs.String("run", "", "gold run key to augment")
	review := fs.Int64("review", 0, "doc-review run id (kb.doc_review_runs.id) of the same document")
	out := fs.String("out", "", "augmentation pool file to write")
	cfgPath := fs.String("config", "doc-review.local.toml", "config file")
	fs.Parse(args)
	if *key == "" || *review <= 0 || *out == "" {
		return usageError("augment needs -run, -review and -out")
	}
	ctx := context.Background()
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		return err
	}
	db, err := complianceeval.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()
	run, err := complianceeval.LoadStoredRun(ctx, db, *key)
	if err != nil {
		return err
	}
	if _, err := listedDoc(cfg, *cfgPath, run.RecordID); err != nil {
		return err
	}
	if _, err := verifySnapshot(ctx, db, run.SnapshotID); err != nil {
		return err
	}
	members, err := complianceeval.SnapshotMembers(ctx, db, run.SnapshotID)
	if err != nil {
		return err
	}
	inSnapshot := map[int64]bool{}
	for _, m := range members {
		inSnapshot[m.RecordID] = true
	}
	copies, err := docreviews.SameMD5RecordIDs(ctx, db, run.RecordID)
	if err != nil {
		return err
	}
	exclude := map[int64]bool{}
	for _, id := range copies {
		exclude[id] = true
	}
	rows, err := complianceeval.LoadReviewRows(ctx, db, *review)
	if err != nil {
		return err
	}
	if rows[0].Record != run.RecordID {
		return fmt.Errorf("review run %d is of record %d, gold run %s of record %d", *review, rows[0].Record, *key, run.RecordID)
	}
	chunks, err := docreviews.LoadGoldChunks(ctx, db, run.RecordID)
	if err != nil {
		return err
	}
	in := make([]complianceeval.AgentActivity, len(run.Activities))
	for i, a := range run.Activities {
		in[i] = complianceeval.AgentActivity{Activity: a.Name, Object: a.Object, LineStart: a.LineStart, LineEnd: a.LineEnd,
			DocumentLevel: a.DocumentLevel, Queries: a.Queries, CitedDocs: a.CitedDocs}
	}
	acts, err := complianceeval.CheckActivities(chunks, in)
	if err != nil {
		return fmt.Errorf("gold run %s: %w", *key, err)
	}
	scfg := searchengine.LoadConfig()
	load := func(refs []searchengine.ArtifactRef) (map[string]complianceeval.Candidate, error) {
		res, err := searchengine.Load(ctx, db, scfg, refs)
		if err != nil {
			return nil, err
		}
		out := map[string]complianceeval.Candidate{}
		for _, r := range res {
			src := r.SourceTitle
			if src == "" {
				src = r.SourceFilename
			}
			out[r.ArtifactType+"/"+r.ArtifactID] = complianceeval.Candidate{ArtifactType: r.ArtifactType, ArtifactID: r.ArtifactID,
				RecordID: r.InputRecordID, Source: src, Text: complianceeval.CandidateText(r.Snippet)}
		}
		return out, nil
	}
	pf, n, err := complianceeval.AugmentPool(run, acts, *review, rows, inSnapshot, exclude, load)
	if err != nil {
		return err
	}
	if n == 0 {
		fmt.Printf("nothing to augment: gold run %s already judged every passage review run %d reports on snapshot records for the activities it covers\n", *key, *review)
		return nil
	}
	if err := writePoolFile(*out, pf); err != nil {
		return err
	}
	fmt.Printf("wrote %s: %d passages review run %d reported that gold run %s never judged; label them and save to create the augmented run\n",
		*out, n, *review, *key)
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
	pf, err := readPoolFile(*poolPath)
	if err != nil {
		return err
	}
	doc, err := listedDoc(cfg, *cfgPath, pf.RecordID)
	if err != nil {
		return err
	}
	rules, err := complianceeval.LoadRulesVersion(*rulesDir, *rulesVersion)
	if err != nil {
		return err
	}
	labels, err := readLabels(*labelsPath)
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
	acts, err := complianceeval.CheckActivities(chunks, poolActivities(pf))
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
	if pf.BaseRun != "" {
		// An augmentation pool: the new run is the base run plus the labels of the added passages (design D12).
		base, err := complianceeval.LoadStoredRun(ctx, db, pf.BaseRun)
		if err != nil {
			return err
		}
		if base.RulesVersion != rules.Version {
			return fmt.Errorf("gold run %s used rules %s; label the augmentation under the same version, not %s", base.Key, base.RulesVersion, rules.Version)
		}
		merged, err := complianceeval.MergeAugmentation(base, pf, gd)
		if err != nil {
			return err
		}
		note := fmt.Sprintf("Augmentation of gold run %s (labelled by %s) with the unjudged passages of review run %d; the added passages were labelled by %s.",
			base.Key, base.Annotator1, pf.ReviewRun, run.Profiles.Annotator1)
		run.AuditNotes = strings.TrimSpace(note + "\n\n" + audit)
		run.Documents = []*complianceeval.GoldDocument{merged}
		run.MaxCandidates, run.BaseRunID, run.ReviewRunID = base.MaxCandidates, base.ID, pf.ReviewRun
	}
	key, id, err := complianceeval.SaveRun(ctx, db, run, time.Now())
	if err != nil {
		return err
	}
	how := "labelled by " + run.Profiles.Annotator1 + " alone (agent)"
	if pf.BaseRun != "" {
		how = fmt.Sprintf("augmented from %s with %d new labels from review run %d", pf.BaseRun, len(labels), pf.ReviewRun)
	}
	fmt.Printf("saved gold run %s (id %d): record %d, %d activities, %d labels in this file; %s, rules %s\n",
		key, id, pf.RecordID, len(run.Documents[0].Activities), len(labels), how, rules.File)
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
	fmt.Printf("  governs labels by where the passage was found: pool %d, cited %d, expand %d, review %d, unknown %d\n",
		s.GovernsByChannel["pool"], s.GovernsByChannel["cited"], s.GovernsByChannel["expand"], s.GovernsByChannel["review"], s.GovernsByChannel["unknown"])
	if s.BaseRun != "" {
		fmt.Printf("  augmented from gold run %s with review run %d\n", s.BaseRun, s.ReviewRun)
	}
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
