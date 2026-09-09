package productreviews

import (
	"context"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

// logger is the package logger (CLAUDE.md "Logs"): a unique loc string.
var logger = loggerutil.CreateDefaultLogger("20260909-PMR")

// Builder composes the profile-construction passes in order:
// propose → ground → expand → attach aspects. It is the seam the profile
// create/rebuild handler and the e2e check both call.
type Builder struct {
	Store    Store
	Proposer Proposer
	Grounder Grounder
	Expander Expander
	Config   *Config
}

// Build runs all four passes against an existing profile. A propose failure
// (CWB_KB_PMR_011) leaves the profile untouched; later passes only ever add
// grounding refs or graph-expanded nodes.
func (b Builder) Build(ctx context.Context, profileID int64, in ProposeInput) error {
	if err := b.Proposer.Propose(ctx, profileID, in); err != nil {
		logger.Warn("product profile propose pass failed", "profile_id", profileID, "error", err)
		return err
	}
	if err := b.Grounder.Ground(ctx, profileID); err != nil {
		return err
	}
	if err := b.Expander.Expand(ctx, profileID); err != nil {
		return err
	}
	if _, err := b.Store.AttachAspects(ctx, profileID, b.Config); err != nil {
		return err
	}
	return nil
}
