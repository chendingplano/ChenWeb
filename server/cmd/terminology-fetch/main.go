// Command terminology-fetch downloads the freely available external
// terminology resources (QUDT, UCUM, BIPM SIRP, Wikidata pilot subset) into a
// local directory as immutable artifacts, computing SHA-256 and writing an
// unapproved draft manifest (license_review_status=pending_review) for each.
// It is the network-enabled bootstrap step: terminology-import itself never
// fetches live URLs. Permission-gated resources (IEC 60050-845) are refused.
//
// Usage:
//
//	terminology-fetch list
//	terminology-fetch status [--dir <dir>] [--source <id>]
//	terminology-fetch fetch --source <id> --dir <dir> [--titles a,b,c]
//
// The directory defaults to TERMINOLOGY_DIR, else <DATA_HOME_DIR>/terminology.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/terminology"
)

type app interface {
	resources() []terminology.Resource
	status(dir string, id terminology.ResourceID) (terminology.FetchStatus, error)
	fetch(ctx context.Context, dir string, id terminology.ResourceID, opts ...terminology.FetchOption) (terminology.FetchStatus, error)
}

type defaultApp struct{}

func (defaultApp) resources() []terminology.Resource { return terminology.Resources() }
func (defaultApp) status(dir string, id terminology.ResourceID) (terminology.FetchStatus, error) {
	return terminology.ReadStatus(dir, id)
}
func (defaultApp) fetch(ctx context.Context, dir string, id terminology.ResourceID, opts ...terminology.FetchOption) (terminology.FetchStatus, error) {
	return terminology.Fetch(ctx, id, dir, opts...)
}

func main() {
	code := execute(context.Background(), os.Args[1:], os.Stdout, os.Stderr, defaultApp{})
	os.Exit(code)
}

func resolveDir(flagDir string) (string, error) {
	if d := strings.TrimSpace(flagDir); d != "" {
		return d, nil
	}
	if d := strings.TrimSpace(os.Getenv("TERMINOLOGY_DIR")); d != "" {
		return d, nil
	}
	if home := strings.TrimSpace(os.Getenv("DATA_HOME_DIR")); home != "" {
		return filepath.Join(home, "terminology"), nil
	}
	return "", fmt.Errorf("--dir, TERMINOLOGY_DIR, or DATA_HOME_DIR is required")
}

func execute(ctx context.Context, args []string, stdout, stderr io.Writer, application app) int {
	if len(args) == 0 {
		writeUsage(stderr)
		return 2
	}
	switch args[0] {
	case "list":
		return runList(stdout, stderr, application)
	case "status":
		return runStatus(ctx, args[1:], stdout, stderr, application)
	case "fetch":
		return runFetch(ctx, args[1:], stdout, stderr, application)
	case "help", "-h", "--help":
		writeUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown subcommand %q\n", args[0])
		writeUsage(stderr)
		return 2
	}
}

func runList(stdout, stderr io.Writer, application app) int {
	if err := json.NewEncoder(stdout).Encode(application.resources()); err != nil {
		fmt.Fprintf(stderr, "encode: %v\n", err)
		return 1
	}
	return 0
}

func runStatus(ctx context.Context, args []string, stdout, stderr io.Writer, application app) int {
	fs := flag.NewFlagSet("terminology-fetch status", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dirFlag := fs.String("dir", "", "storage directory (default TERMINOLOGY_DIR or DATA_HOME_DIR/terminology)")
	sourceFlag := fs.String("source", "", "one resource id (default: all)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dir, err := resolveDir(*dirFlag)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	if *sourceFlag != "" {
		st, err := application.status(dir, terminology.ResourceID(*sourceFlag))
		if err != nil {
			fmt.Fprintf(stderr, "status: %v\n", err)
			return 1
		}
		return writeJSON(stdout, st)
	}
	statuses := []terminology.FetchStatus{}
	for _, r := range application.resources() {
		st, err := application.status(dir, r.ID)
		if err != nil {
			fmt.Fprintf(stderr, "status %s: %v\n", r.ID, err)
			return 1
		}
		statuses = append(statuses, st)
	}
	return writeJSON(stdout, statuses)
}

func runFetch(ctx context.Context, args []string, stdout, stderr io.Writer, application app) int {
	fs := flag.NewFlagSet("terminology-fetch fetch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dirFlag := fs.String("dir", "", "storage directory (default TERMINOLOGY_DIR or DATA_HOME_DIR/terminology)")
	sourceFlag := fs.String("source", "", "resource id to download (required)")
	titlesFlag := fs.String("titles", "", "wikidata enwiki titles, pipe-separated (default: pilot corpus)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	source := strings.TrimSpace(*sourceFlag)
	if source == "" {
		fmt.Fprintln(stderr, "--source is required")
		return 2
	}
	dir, err := resolveDir(*dirFlag)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	opts := []terminology.FetchOption{terminology.WithClient(&http.Client{Timeout: 10 * time.Minute})}
	if *titlesFlag != "" {
		opts = append(opts, terminology.WithWikidataTitles(strings.Split(*titlesFlag, "|")))
	}
	st, err := application.fetch(ctx, dir, terminology.ResourceID(source), opts...)
	if err != nil {
		fmt.Fprintf(stderr, "fetch %s: %v\n", source, err)
		return 1
	}
	return writeJSON(stdout, st)
}

func writeJSON(w io.Writer, v any) int {
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return 1
	}
	return 0
}

func writeUsage(w io.Writer) {
	fmt.Fprintln(w, `usage: terminology-fetch <command> [flags]

commands:
  list                           print the resource catalog as JSON
  status --dir <dir> [--source <id>]
                                 print persisted download status (one or all)
  fetch --source <id> --dir <dir> [--titles a,b,c]
                                 download one freely available resource

resources: qudt, ucum, sirp, wikidata (free); iec-60050-845 (permission-gated)
directory: --dir, else TERMINOLOGY_DIR, else <DATA_HOME_DIR>/terminology`)
}
