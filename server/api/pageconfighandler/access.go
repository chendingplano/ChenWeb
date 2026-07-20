package pageconfighandler

import "strings"

// isAuthorized implements the accessibility model from spec 2026072001 §9.2,
// evaluated on an entry's default-language row. An entry is accessible iff:
//   - enabled is true, AND
//   - accessible is true, AND
//   - access_role is non-empty and contains at least one VALID role (a key in
//     validRoles), AND
//   - the user holds at least one of those roles.
//
// Matching is case-insensitive. There is no wildcard: an empty, null, or
// all-invalid access_role suspends the entry (inaccessible to everyone), and a
// disabled or non-accessible entry is inaccessible to everyone. Fail closed.
func isAuthorized(defaultRow configRow, userRoles []string, validRoles []string) bool {
	if !defaultRow.Enabled || !defaultRow.Accessible {
		return false
	}

	valid := toLowerSet(validRoles)
	user := toLowerSet(userRoles)

	hasValidRole := false
	for _, r := range defaultRow.AccessRole {
		key := strings.ToLower(strings.TrimSpace(r))
		if key == "" || !valid[key] {
			continue
		}
		hasValidRole = true
		if user[key] {
			return true
		}
	}
	// access_role had no valid role at all -> suspended (already false);
	// or it had valid roles but the user holds none of them.
	_ = hasValidRole
	return false
}

func toLowerSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		key := strings.ToLower(strings.TrimSpace(item))
		if key != "" {
			out[key] = true
		}
	}
	return out
}
