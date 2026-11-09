// Command docbuilder-classify classifies the unclassified documents of one Document Builder
// source with an LLM and saves the result in <dir>/.docbuilder/manifest.json. Files that already
// have a category are left alone, so this is run once per batch of new files.
//
//	docbuilder-classify -source user-manuals [-config docbuilder.local.toml] [-model profile] [-dry-run] [-force]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/deepdoc/server/api/docbuilder"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func main() {
	cfg := flag.String("config", "docbuilder.local.toml", "sources file")
	key := flag.String("source", "", "source key in the sources file (required)")
	model := flag.String("model", os.Getenv("DOCBUILDER_MODEL"), "model profile in .models.toml (default deepseek-flash-processor)")
	promptRef := flag.String("prompt", "prompt-docbuilder-classify-v1.md", "classification prompt file in prompts/")
	dry := flag.Bool("dry-run", false, "scan and report what would be classified; no model call, nothing saved")
	force := flag.Bool("force", false, "reclassify files that already have a category")
	flag.Parse()
	if err := run(*cfg, *key, *model, *promptRef, *dry, *force); err != nil {
		fmt.Fprintln(os.Stderr, "docbuilder-classify:", err)
		os.Exit(1)
	}
}

func run(cfgPath, key, model, promptRef string, dry, force bool) error {
	sources, err := docbuilder.LoadSources(cfgPath)
	if err != nil {
		return err
	}
	src, ok := sources[key]
	if !ok {
		return fmt.Errorf("source %q not found in %s", key, cfgPath)
	}
	m, err := docbuilder.LoadManifest(src.Dir)
	if err != nil {
		return err
	}
	if err := docbuilder.Refresh(src.Dir, m); err != nil {
		return err
	}
	pending := 0
	for _, e := range m.Entries {
		if e.Category == "" || force {
			pending++
		}
	}
	fmt.Printf("%s: %d files, %d to classify\n", src.Dir, len(m.Entries), pending)
	if dry || pending == 0 {
		return nil
	}
	if model == "" {
		model = "deepseek-flash-processor"
	}
	logger := loggerutil.CreateDefaultLogger("20261109-701")
	client, modelName, err := docprocessing.BuildReviewerToolClient(model, logger)
	if err != nil {
		return err
	}
	prompt, _, _, err := docprocessing.LoadPromptByRef(promptRef)
	if err != nil {
		return err
	}
	n, err := docbuilder.ClassifyNew(context.Background(), src.Dir, m,
		&docbuilder.LLMClassifier{Client: client, Model: modelName, Prompt: prompt}, model, force)
	fmt.Printf("classified %d files\n", n)
	return err
}
