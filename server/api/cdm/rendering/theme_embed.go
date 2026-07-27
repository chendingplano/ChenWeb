package rendering

import _ "embed"

// DefaultTheme is the Phase 1 fallback Typst template (spec §5.3/§5.4),
// embedded so a deployed binary carries it rather than depending on the
// source tree being present alongside it at runtime.
//
//go:embed theme.typ
var DefaultTheme []byte
