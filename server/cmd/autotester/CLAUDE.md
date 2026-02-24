# Auto Tester

## CLI Entry Point:

- cmd/autotester/main.go - Main entry point with CLI flags
- cmd/autotester/registry.go - Registers all testers

## Available Auto Testers:

- api/autotesters/user_tester.go - Full user API tester with CRUD operations
- api/autotesters/project_tester.go - Project tester (stub)
- api/autotesters/document_tester.go - Document tester (stub)
- api/autotesters/email_tester.go - Email sender tester (stub)

## Usage
### Mise
```bash
# Run all tests
mise run-all-tests

# Run specific testers
go run ./server/cmd/autotester/ --tester=user_tester,database_tester

# Run with parallel execution
go run ./server/cmd/autotester/ --parallel --max-parallel=8

# Run with specific seed for reproducibility
go run ./server/cmd/autotester/ --seed=8675309

# Run smoke tests only
go run ./server/cmd/autotester/ --purpose=smoke

# Write JSON report
go run ./server/cmd/autotester/ --json-report=/tmp/report.json
```

### CLI
```bash
# Run all tests
go run ./server/cmd/autotester/

# Run specific testers
go run ./server/cmd/autotester/ --tester=user_tester,database_tester

# Run with parallel execution
go run ./server/cmd/autotester/ --parallel --max-parallel=8

# Run with specific seed for reproducibility
go run ./server/cmd/autotester/ --seed=8675309

# Run smoke tests only
go run ./server/cmd/autotester/ --purpose=smoke

# Write JSON report
go run ./server/cmd/autotester/ --json-report=/tmp/report.json
```