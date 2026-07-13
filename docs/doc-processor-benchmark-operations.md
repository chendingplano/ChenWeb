# Document-processor benchmark operations

Validate an experiment before execution:

```sh
go run ./server/cmd/doc-benchmark validate --experiment benchmark/doc-processors/experiments/example.toml
```

Datasets are immutable and addressed by ID/version. Runs record request, dataset, case-set, runtime, and executable hashes. Workers use framed JSON lines and are isolated from the command process. Failed processor output may be retained for verified rescore; unverified work is disposable and must be cleaned with an explicit attempt ID. Never delete verified evidence as routine cleanup.

Database/live integration is opt-in and requires `TEST_DATABASE_URL`; production LLM execution additionally requires `BENCHMARK_LIVE_INTEGRATION=1`. Cancellation is terminal and should be resumed with a new attempt after inspecting the recorded failure class.
