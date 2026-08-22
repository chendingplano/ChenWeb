// Package assertions_test (external, black-box) rather than assertions:
// keywords already imports assertions (production code), so an internal
// (package assertions) test file importing keywords back would be an import
// cycle. An external test file compiled into the same test binary can import
// keywords safely, since keywords only ever imports the plain, non-test
// build of assertions.
package assertions_test

// Blank-imported so its init() registers the real ClassSynthesizer
// (metric-class-synthesis-seam) for this package's test binary. Without
// this, every writeMetricLossless integration test that reaches
// SynthesizeClass fails with "no class synthesizer registered" when this
// package's tests run in isolation (as opposed to as part of a larger
// binary, such as the server's own, that already imports keywords).
import (
	_ "github.com/chendingplano/deepdoc/server/api/ontology/keywords"
)
