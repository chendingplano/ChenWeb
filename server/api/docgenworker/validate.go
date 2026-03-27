package docgenworker

import (
	"encoding/json"
	"fmt"
	"strings"
)

var requiredConverterValues = []string{"customer_id", "customer_name", "email"}

// ValidateSQLStatement returns an error if stmt is not a SELECT query.
func ValidateSQLStatement(stmt string) error {
	trimmed := strings.TrimSpace(strings.ToUpper(stmt))
	if trimmed == "" {
		return fmt.Errorf("sql_statement must not be empty (CWB_DGW_050)")
	}
	if !strings.HasPrefix(trimmed, "SELECT") {
		return fmt.Errorf("sql_statement must be a SELECT query (CWB_DGW_055)")
	}
	return nil
}

// ValidateConverter parses converterJSON and verifies that the required
// log fields (customer_id, customer_name, email) appear as values.
// Returns the parsed map on success.
func ValidateConverter(converterJSON string) (map[string]string, error) {
	var m map[string]string
	if err := json.Unmarshal([]byte(converterJSON), &m); err != nil {
		return nil, fmt.Errorf("converter must be valid JSON object: %w (CWB_DGW_060)", err)
	}
	valueSet := make(map[string]bool)
	for _, v := range m {
		valueSet[v] = true
	}
	for _, req := range requiredConverterValues {
		if !valueSet[req] {
			return nil, fmt.Errorf("converter missing required mapping to %q (CWB_DGW_065)", req)
		}
	}
	return m, nil
}
