package profiles

import (
	"strconv"

	"github.com/chendingplano/deepdoc/server/api/ontology/semrules"
)

type ReviewApplicabilityContext struct {
	AsOfDate         string
	Jurisdiction     string
	OperatingContext string
	Purpose          string
	ReleaseID        int64
}

func BuildReviewContextFacts(ctx ReviewApplicabilityContext) (semrules.FactSet, error) {
	releaseID := ""
	if ctx.ReleaseID != 0 {
		releaseID = strconv.FormatInt(ctx.ReleaseID, 10)
	}
	builder := semrules.NewFactSetBuilder()
	for _, fact := range []semrules.Fact{
		reviewFact("review.as_of", ctx.AsOfDate, ""),
		reviewFact("review.jurisdiction", ctx.Jurisdiction, releaseID),
		reviewFact("review.operating_context", ctx.OperatingContext, ""),
		reviewFact("review.purpose", ctx.Purpose, releaseID),
	} {
		if err := builder.Add(fact); err != nil {
			return nil, err
		}
	}
	return builder.Build(), nil
}

func reviewFact(path, value, releaseID string) semrules.Fact {
	fact := semrules.Fact{Path: path, State: semrules.FactMissing}
	if value == "" {
		return fact
	}
	fact.State = semrules.FactKnown
	fact.Value = value
	fact.ReleaseID = releaseID
	return fact
}
