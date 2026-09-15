package agentservicehandler

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPilotEvaluationCasesCoverBothGuidesAndSafetyScenarios(t *testing.T) {
	data, err := os.ReadFile("testdata/evaluation-cases.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Profile  string `json:"profile"`
		Scenario string `json:"scenario"`
		Question string `json:"question"`
		Expected string `json:"expected"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	required := []string{"normal", "ambiguous", "missing_evidence", "conflicting_sources", "hostile_document", "access_denied", "access_revoked_after_save"}
	for _, profile := range []string{"knowledge-guide", "problem-diagnostics"} {
		for _, scenario := range required {
			found := false
			for _, item := range cases {
				if item.Profile == profile && item.Scenario == scenario && item.Question != "" && item.Expected != "" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("missing evaluation case for %s / %s", profile, scenario)
			}
		}
	}
}
