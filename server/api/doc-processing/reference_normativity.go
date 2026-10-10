package docprocessing

// Normativity and list linking for one document's references (design D3, devdoc §3.5).
// Normativity comes only from the document's formal lists, never from wording: a resource in the
// normative references list is normative wherever it is cited; one only in an informative list
// (bibliography, references) is informative; anything else is unlisted. Wording that binds is
// recorded separately as in_requirement.

import (
	"strings"
	"unicode"
)

// Values of list_mismatch.
const (
	refMismatchNotListed  = "not_listed"
	refMismatchNeverCited = "never_cited"
)

// labelMechanisms are citations that only point at an entry of the document's own list.
var labelMechanisms = refSetOf("numbered_citation", "author_year", "citation_key", "footnote")

// deriveRefNormativity fills ListEntryIdx, Normativity and ListMismatch on every mention of one
// document. Label citations ("[12]", "Smith et al., 2024") first inherit the identifiers and
// title of the list entry they point at.
func deriveRefNormativity(ms []RefMention) {
	labelIdx := map[string]int{}
	for i := range ms {
		ms[i].ListEntryIdx = -1
		if ms[i].RefForm == refFormListed && !ms[i].Internal {
			if l := refNormLabel(ms[i].ListLabel); l != "" {
				labelIdx[l] = i
			}
		}
	}
	for i := range ms {
		m := &ms[i]
		if m.RefForm != refFormInText || m.Internal || len(m.Identifiers) > 0 || !labelMechanisms[m.Mechanism] {
			continue
		}
		j, ok := labelIdx[refNormLabel(m.CitedRaw)]
		if !ok {
			continue
		}
		entry := ms[j]
		m.Identifiers = append([]RefIdentifier(nil), entry.Identifiers...)
		if m.TargetTitle == "" {
			m.TargetTitle = entry.TargetTitle
		}
		if m.TargetVersion == "" {
			m.TargetVersion = entry.TargetVersion
		}
		m.TargetType, m.TargetSubtype, m.TargetSubtypeLabel = entry.TargetType, entry.TargetSubtype, entry.TargetSubtypeLabel
		m.ListEntryIdx = j
	}

	normative := map[string]int{}
	informative := map[string]int{}
	for i, m := range ms {
		if m.RefForm != refFormListed || m.Internal {
			continue
		}
		key, _, _, ok := refIdentityKey(m.Identifiers, m.TargetTitle)
		if !ok {
			continue
		}
		if m.ListKind == refListNormative {
			if _, dup := normative[key]; !dup {
				normative[key] = i
			}
		} else if _, dup := informative[key]; !dup {
			informative[key] = i
		}
	}

	used := map[int]bool{}
	for i := range ms {
		m := &ms[i]
		if m.RefForm == refFormListed {
			if m.ListKind == refListNormative {
				m.Normativity = refNormativityListed
			} else {
				m.Normativity = refNormativityInform
			}
			continue
		}
		m.Normativity = refNormativityNone
		if m.Internal {
			continue
		}
		if m.ListEntryIdx >= 0 { // a label citation already linked to its entry
			if ms[m.ListEntryIdx].ListKind == refListNormative {
				m.Normativity = refNormativityListed
			} else {
				m.Normativity = refNormativityInform
			}
		} else if key, _, _, ok := refIdentityKey(m.Identifiers, m.TargetTitle); ok {
			if j, hit := normative[key]; hit {
				m.Normativity, m.ListEntryIdx = refNormativityListed, j
			} else if j, hit := informative[key]; hit {
				m.Normativity, m.ListEntryIdx = refNormativityInform, j
			}
		}
		if m.ListEntryIdx >= 0 {
			used[m.ListEntryIdx] = true
		}
	}

	if len(normative) == 0 {
		return // mismatches are defined only for documents with a normative references list
	}
	for i := range ms {
		m := &ms[i]
		switch {
		case m.RefForm == refFormInText && !m.Internal && m.InRequirement && m.Normativity != refNormativityListed && refListable(*m):
			m.ListMismatch = refMismatchNotListed
		case m.RefForm == refFormListed && m.ListKind == refListNormative && !used[i]:
			m.ListMismatch = refMismatchNeverCited
		}
	}
}

// refListable reports whether a citation names a document that could stand in a normative
// references list: one with a public identifier. Manufacturer's instructions or "the supplier's
// data" cannot, so their absence from the list is no drafting defect.
func refListable(m RefMention) bool {
	_, _, public, ok := refIdentityKey(m.Identifiers, "")
	return ok && public
}

// refNormLabel normalizes a list label or label citation: "[12]" and "12." -> "12";
// "(Smith et al., 2024)" and "Smith et al. 2024" -> "smith 2024".
func refNormLabel(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, "et al", " "))
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
