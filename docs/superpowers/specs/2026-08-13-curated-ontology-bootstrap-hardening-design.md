# Curated Ontology Bootstrap Hardening

## Goal

Make the curated ontology bootstrap safe for fresh databases and routine
curated-vocabulary maintenance without changing an existing active release.

## Design

`EnsureCuratedModules` will strictly install `core` and `document-authority`.
It will install `measurement` only after `quantity` has an active release. If
that prerequisite is absent, it will return a structured warning rather than
an error, allowing the service to start and the QUDT import path to create the
dependency. A later service start installs measurement automatically.

The seed package will derive each curated release version from a deterministic
hash of the Go-defined module content. A source edit therefore creates a new
release instead of reusing `1.0.0`. A new seed release is activated only when
the module has no active release; an existing active release is never replaced
at startup. The existing runtime gate sees the newly included terms even when
an operator must later choose to activate the newer release.

For a changed preferred label, bootstrap will supersede the current label
before creating the replacement rather than attempting a second current
`prefLabel`. The operation remains idempotent. A dedicated three-minute
context isolates Deepdoc bootstrap from its 30-second process setup context.

The CLI will choose `PG_USER_NAME`, then `PG_USER`, then the literal default
`cding`. Its existing command behavior remains strict: an explicitly invoked
seed reports unreleasable dependencies as an error. The `mise` task will use
the same host default and describe the current CLI behavior accurately.

## Acceptance criteria

- A fresh migrated database starts with core and document-authority installed,
  while reporting measurement as deferred until quantity is active.
- A missing or changed curated label does not cause startup failure.
- A content edit results in a new curated release; startup does not replace an
  active release.
- Deepdoc gives the bootstrap a three-minute timeout.
- CLI user resolution returns `cding` when neither PostgreSQL user variable is
  set.
- Behavioral package tests cover prerequisite deferral, label evolution,
  release versioning/activation, and environment resolution.
