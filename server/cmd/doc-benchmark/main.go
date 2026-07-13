package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	docbenchmark "github.com/chendingplano/deepdoc/server/api/doc-benchmark"
)

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "validate":
		if err := validate(os.Args[2:]); err != nil {
			emitError("validation_error", err)
			os.Exit(2)
		}
	default:
		emitError("usage_error", fmt.Errorf("unknown command %q", os.Args[1]))
		os.Exit(2)
	}
}

func validate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	experiment := fs.String("experiment", "", "experiment TOML path")
	datasets := fs.String("datasets-root", "benchmark/doc-processors/datasets", "dataset root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *experiment == "" {
		return fmt.Errorf("--experiment is required")
	}
	b, err := os.ReadFile(*experiment)
	if err != nil {
		return err
	}
	e, err := docbenchmark.LoadExperiment(b, *datasets)
	if err != nil {
		return err
	}
	processors := map[string]string{}
	for p, h := range e.ProcessorCaseSetHashes {
		processors[string(p)] = h
	}
	out := map[string]any{"dataset_id": e.DatasetID, "dataset_version": e.DatasetVersion, "dataset_hash": e.DatasetHash, "request_hash": e.RequestHash, "processor_case_set_hashes": processors}
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

func emitError(code string, err error) {
	var e errorEnvelope
	e.Error.Code = code
	e.Error.Message = err.Error()
	_ = json.NewEncoder(os.Stderr).Encode(e)
}
func usage() {
	fmt.Fprintln(os.Stderr, "usage: doc-benchmark validate --experiment FILE [--datasets-root DIR]")
}
