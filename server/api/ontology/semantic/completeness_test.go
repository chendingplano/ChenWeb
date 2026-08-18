package semantic

import (
	"reflect"
	"testing"
)

func TestCompletenessReportExposesArtifactsWithMissingValue(t *testing.T) {
	if _, ok := reflect.TypeOf(CompletenessReport{}).FieldByName("ArtifactsWithMissingValue"); !ok {
		t.Fatal("completeness report must distinguish present artifacts with missing values")
	}
}
