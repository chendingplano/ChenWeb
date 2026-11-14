// Command compliance-score scores saved Compliance Review runs against a gold run
// (openspec compliance-reviewer-phase2).
//
//	mise exec -- go run ./server/cmd/compliance-score -gold 20261114_150000,20261114_160000 -runs 101,102
//	mise exec -- go run ./server/cmd/compliance-score -compare 101,120      # activity-list stability of one document
//
// -gold is one or more gold run keys sharing one snapshot (agent-built gold has one run per document);
// -runs are doc-review run ids (kb.doc_review_runs.id), one per document. Gold labels come from the
// coding agent or a model panel, never from legal validation; the output names which.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	complianceeval "github.com/chendingplano/deepdoc/server/api/compliance-eval"
)

func main() {
	gold := flag.String("gold", "", "comma-separated gold run keys (testbed.compliance_gold_runs.run_key)")
	runs := flag.String("runs", "", "comma-separated doc-review run ids to score")
	compare := flag.String("compare", "", "two doc-review run ids of the same document: are the activity lists identical?")
	flag.Parse()
	if err := run(*gold, *runs, *compare); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func ids(s string) ([]int64, error) {
	var out []int64
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		n, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bad run id %q", p)
		}
		out = append(out, n)
	}
	return out, nil
}

func run(gold, runs, compare string) error {
	ctx := context.Background()
	db, err := complianceeval.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()
	if compare != "" {
		pair, err := ids(compare)
		if err != nil || len(pair) != 2 {
			return fmt.Errorf("-compare needs exactly two run ids")
		}
		a, err := complianceeval.LoadReviewRows(ctx, db, pair[0])
		if err != nil {
			return err
		}
		b, err := complianceeval.LoadReviewRows(ctx, db, pair[1])
		if err != nil {
			return err
		}
		same, diffs := complianceeval.ActivityListsEqual(a, b)
		if same {
			fmt.Printf("activity lists identical (%d activities)\n", len(a))
			return nil
		}
		for _, d := range diffs {
			fmt.Println(d)
		}
		os.Exit(1)
	}
	if gold == "" || runs == "" {
		return fmt.Errorf("usage: compliance-score -gold KEY -runs 1,2 | -compare A,B")
	}
	rids, err := ids(runs)
	if err != nil {
		return err
	}
	var keys []string
	for _, k := range strings.Split(gold, ",") {
		if k = strings.TrimSpace(k); k != "" {
			keys = append(keys, k)
		}
	}
	acts, infos, inSnapshot, err := complianceeval.LoadGold(ctx, db, keys)
	if err != nil {
		return err
	}
	var rows []complianceeval.ReviewRow
	scored := map[int64]bool{}
	for _, id := range rids {
		rs, err := complianceeval.LoadReviewRows(ctx, db, id)
		if err != nil {
			return err
		}
		rows = append(rows, rs...)
		scored[rs[0].Record] = true
	}
	var goldScored []complianceeval.GoldAct
	for _, a := range acts {
		if scored[a.Record] {
			goldScored = append(goldScored, a)
		}
	}
	if len(goldScored) == 0 {
		return fmt.Errorf("none of the scored runs belongs to a document of gold run(s) %s", gold)
	}
	m := complianceeval.Score(goldScored, rows, inSnapshot)
	for _, in := range infos {
		how := "LLM-adjudicated labels (model panel)"
		if in.Method == complianceeval.MethodAgent {
			how = "labelled by " + in.Annotator1 + " alone (agent; no inter-annotator agreement)"
		}
		fmt.Printf("gold run %s, snapshot %d: %s\n", in.Key, in.SnapshotID, how)
	}
	fmt.Printf("%d document(s) scored\n", len(scored))
	fmt.Printf("  activities: gold %d (covered by a reviewer row: %d), reviewer rows %d (supported by gold: %d)\n", m.GoldActivities, m.MatchedActivities, m.ReviewRows, m.SupportedRows)
	fmt.Printf("  activity precision:               %s\n", m.ActivityPrecision)
	fmt.Printf("  activity recall:                  %s\n", m.ActivityRecall)
	fmt.Printf("  grounding recall within pool:     %s   (an upper bound: records the pool missed are unjudged)\n", m.GroundingRecall)
	fmt.Printf("  'no grounding' precision:         %s\n", m.NoGroundingPrecision)
	fmt.Printf("  'no grounding' missed by search:  %s\n", m.NoGroundingMissedBySearch)
	fmt.Printf("  false grounding (none expected):  %s\n", m.FalseGrounding)
	fmt.Printf("  unjudged, on records added after the snapshot: %d activities left out of false grounding, %d candidate records\n", m.UnjudgedActivities, m.UnjudgedCandidates)
	fmt.Printf("  ambiguous judgements (not used):  %d\n", m.AmbiguousJudgements)
	fmt.Printf("  headline (mean of F1, recall, 1-false grounding; summary only): %s\n", m.Headline)
	return nil
}
