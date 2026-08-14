# PDF Parser JetStream Recovery Design

## Goal

Allow `ChenWeb/python/pdf-parser` to restart safely after a machine crash when
JetStream streams already exist under names different from the current startup
defaults.

## Design

During startup, each configured stream name is checked first. If it does not
exist, the service searches JetStream for a stream already owning the required
subject and reuses that stream. Only when no stream owns the subject does it
create the configured stream. A stream that owns a different subject is not
silently reused.

JetStream errors are classified narrowly: only a not-found response permits
fallback or creation. Authentication, authorization, timeout, and other API
errors propagate unchanged so startup diagnostics identify the real failure.

The existing environment variables remain the override mechanism. The startup
documentation will explain that stream names are compatibility labels and that
existing streams are discovered by subject.

## Testing

Unit tests will cover reuse of a differently named stream, creation when the
subject is absent, and propagation of non-not-found errors. Existing parser
tests and a startup-level static/configuration check will be run before commit.
