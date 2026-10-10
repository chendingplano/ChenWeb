package docprocessing

// Identifier parsing for extract_references (design D2/D4): exact identifiers are read by code,
// not by the model. An identifier names or locates a resource; the identity key (no edition)
// groups every way of citing the same resource, and the kind decides whether the resource is
// public (shared across customers) or private to the citing store.

import (
	"regexp"
	"strings"
)

// RefIdentifier is one parsed identifier of a cited resource.
type RefIdentifier struct {
	Kind    string `json:"kind"`
	Value   string `json:"value"`
	Edition string `json:"edition,omitempty"`
}

// refPublicKinds are identifier kinds that name a resource publicly (design D4).
var refPublicKinds = refSetOf("standard", "rfc", "legal", "doi", "arxiv", "pmid", "isbn", "issn", "patent", "url")

// refKindPriority orders kinds when choosing a resource's identity key.
var refKindPriority = []string{"standard", "rfc", "legal", "doi", "arxiv", "pmid", "isbn", "issn", "patent", "url"}

var (
	refDOIRe    = regexp.MustCompile(`(?i)\b(10\.\d{4,9}/[^\s"<>]+)`)
	refURLRe    = regexp.MustCompile(`(?i)\bhttps?://[^\s"<>）)]+`)
	refArxivRe  = regexp.MustCompile(`(?i)\barxiv:\s*(\d{4}\.\d{4,5})(v\d+)?`)
	refISBNRe   = regexp.MustCompile(`(?i)\bISBN(?:-1[03])?[:\s]*((?:97[89][-\s]?)?\d[\d\-\s]{8,15}[\dX])`)
	refISSNRe   = regexp.MustCompile(`(?i)\bISSN[:\s]*(\d{4}-\d{3}[\dX])`)
	refPMIDRe   = regexp.MustCompile(`(?i)\bPMID:?\s*(\d{5,9})`)
	refRFCRe    = regexp.MustCompile(`(?i)\bRFC\s*(\d{1,5})\b`)
	refPatentRe = regexp.MustCompile(`\b(US|EP|CN|WO|JP|KR|DE)\s?(\d{6,})\s?([A-Z]\d?)\b`)
	refEURe     = regexp.MustCompile(`(?i)\b(Regulation|Directive|Decision)\s*\((EU|EC|EEC)\)\s*(?:No\.?\s*)?(\d{2,4}/\d+)`)
	refEUDirRe  = regexp.MustCompile(`(?i)\bDirective\s+(\d{2,4}/\d+/(?:EU|EC|EEC))`)
	refCFRRe    = regexp.MustCompile(`(?i)\b(\d{1,2})\s*CFR\s*(?:Part\s*)?(\d+)`)
	refUSCRe    = regexp.MustCompile(`(?i)\b(\d{1,2})\s*U\.?S\.?C\.?\s*§*\s*(\d+)`)
	// refGenericStandardRe is used only when the model says the citation is a standard number
	// and no known prefix matched (CJJ 27, JGJ 120, HJ 2.2).
	refGenericStandardRe = regexp.MustCompile(`\b([A-Z]{1,6}(?:[(（][^)）]{1,4}[)）])?(?:/[A-Z]{1,2})?)\s*(\d+(?:\.\d+)*(?:-\d{1,3}(?:\b|$))*)(?:\s*[-—–:]\s*((?:19|20)\d{2}))?`)
	// refStandardRe: publisher prefix (case-sensitive, standards write it in capitals), number
	// (with part numbers), optional edition year. A 4-digit year after a dash, en/em dash or colon
	// is the edition; "-1" is a part.
	refStandardRe = regexp.MustCompile(`\b(ISO/IEC|ISO/TS|ISO/TR|ISO|IEC/TS|IEC/TR|IEC|EN|BS\s?EN|BS|DIN\s?EN|DIN|ANSI|ASTM|IEEE|SAE|ITU-T|ITU-R|JIS|UL|NFPA|ASME|CISPR|NF|GB/T|GB/Z|GB|DB\d{2}(?:\(J\))?/T|T/[A-Z]{2,10}|[A-Z]{1,4}/[TZ])\s*([A-Z]?\d+(?:\.\d+)*(?:-\d{1,3}(?:\b|$))*)(?:\s*[-—–:]\s*((?:19|20)\d{2}))?`)
)

// parseRefIdentifiers reads every identifier in a citation text. It never guesses: text without
// a recognizable identifier yields none (the resource is then keyed by title).
func parseRefIdentifiers(raw string) []RefIdentifier {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []RefIdentifier
	seen := map[string]bool{}
	add := func(id RefIdentifier) {
		k := id.Kind + "|" + id.Value
		if id.Value == "" || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, id)
	}
	urlSpans := refURLRe.FindAllStringIndex(raw, -1)
	for _, sp := range urlSpans {
		add(RefIdentifier{Kind: "url", Value: strings.TrimRight(raw[sp[0]:sp[1]], ".,;")})
	}
	for _, m := range refDOIRe.FindAllStringSubmatch(raw, -1) {
		add(RefIdentifier{Kind: "doi", Value: strings.ToLower(strings.TrimRight(m[1], ".,;)"))})
	}
	for _, m := range refArxivRe.FindAllStringSubmatch(raw, -1) {
		add(RefIdentifier{Kind: "arxiv", Value: m[1], Edition: strings.TrimPrefix(strings.ToLower(m[2]), "v")})
	}
	for _, m := range refISBNRe.FindAllStringSubmatch(raw, -1) {
		add(RefIdentifier{Kind: "isbn", Value: strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(m[1]))})
	}
	for _, m := range refISSNRe.FindAllStringSubmatch(raw, -1) {
		add(RefIdentifier{Kind: "issn", Value: strings.ToUpper(m[1])})
	}
	for _, m := range refPMIDRe.FindAllStringSubmatch(raw, -1) {
		add(RefIdentifier{Kind: "pmid", Value: m[1]})
	}
	for _, m := range refRFCRe.FindAllStringSubmatch(raw, -1) {
		add(RefIdentifier{Kind: "rfc", Value: "RFC " + strings.TrimLeft(m[1], "0")})
	}
	for _, m := range refPatentRe.FindAllStringSubmatch(raw, -1) {
		add(RefIdentifier{Kind: "patent", Value: m[1] + m[2] + m[3]})
	}
	for _, m := range refEURe.FindAllStringSubmatch(raw, -1) {
		kind := strings.ToLower(m[1])
		add(RefIdentifier{Kind: "legal", Value: strings.ToUpper(kind[:1]) + kind[1:] + " (" + strings.ToUpper(m[2]) + ") " + m[3]})
	}
	for _, m := range refEUDirRe.FindAllStringSubmatch(raw, -1) {
		add(RefIdentifier{Kind: "legal", Value: "Directive " + strings.ToUpper(m[1])})
	}
	for _, m := range refCFRRe.FindAllStringSubmatch(raw, -1) {
		add(RefIdentifier{Kind: "legal", Value: m[1] + " CFR " + m[2]})
	}
	for _, m := range refUSCRe.FindAllStringSubmatch(raw, -1) {
		add(RefIdentifier{Kind: "legal", Value: m[1] + " U.S.C. " + m[2]})
	}
	for _, idx := range refStandardRe.FindAllStringSubmatchIndex(raw, -1) {
		if refInsideSpans(idx[0], urlSpans) {
			continue
		}
		m := refSubmatches(raw, idx)
		prefix := strings.ToUpper(strings.Join(strings.Fields(m[1]), " "))
		add(RefIdentifier{Kind: "standard", Value: prefix + " " + strings.ToUpper(m[2]), Edition: m[3]})
	}
	return out
}

// parseRefIdentifiersForMention parses a mention's citation text. When the model labelled the
// citation a standard number but no known publisher prefix matched, a generic prefix pattern is
// tried on the citation text alone.
func parseRefIdentifiersForMention(mechanism, raw string) []RefIdentifier {
	ids := parseRefIdentifiers(raw)
	if mechanism != "standard_number" {
		return ids
	}
	for _, id := range ids {
		if id.Kind == "standard" {
			return ids
		}
	}
	if idx := refGenericStandardRe.FindStringSubmatchIndex(raw); idx != nil {
		m := refSubmatches(raw, idx)
		ids = append(ids, RefIdentifier{Kind: "standard", Value: m[1] + " " + m[2], Edition: m[3]})
	}
	return ids
}

func refSubmatches(s string, idx []int) []string {
	out := make([]string, len(idx)/2)
	for i := range out {
		if idx[2*i] >= 0 {
			out[i] = s[idx[2*i]:idx[2*i+1]]
		}
	}
	return out
}

func refInsideSpans(pos int, spans [][]int) bool {
	for _, sp := range spans {
		if pos >= sp[0] && pos < sp[1] {
			return true
		}
	}
	return false
}

// refIdentityKey returns the identity key of a mention (design D4) and the edition it cites:
// the highest-priority identifier "<kind>:<value>", else "title:<normalized title>". ok is false
// when the mention names nothing (no identifier, no title).
func refIdentityKey(ids []RefIdentifier, title string) (key, edition string, public, ok bool) {
	for _, kind := range refKindPriority {
		for _, id := range ids {
			if id.Kind == kind {
				return kind + ":" + id.Value, id.Edition, refPublicKinds[kind], true
			}
		}
	}
	if t := refNormKey(title); t != "" {
		return "title:" + t, "", false, true
	}
	return "", "", false, false
}

// refOwnIdentifier finds a document's own identifier (design D5) in doc_no, the identifier the
// model read from the first pages, the title or the filename, in that order. The first
// knownIDs candidates are known to be document numbers, so the generic standard pattern is
// allowed for them (JJG(苏)263-2024).
func refOwnIdentifier(knownIDs int, candidates ...string) (RefIdentifier, bool) {
	for i, c := range candidates {
		c = strings.NewReplacer("_", "/").Replace(strings.TrimSpace(c))
		mech := ""
		if i < knownIDs {
			mech = "standard_number"
		}
		for _, id := range parseRefIdentifiersForMention(mech, c) {
			if id.Kind == "standard" || id.Kind == "rfc" || id.Kind == "legal" || id.Kind == "doi" || id.Kind == "arxiv" {
				return id, true
			}
		}
	}
	return RefIdentifier{}, false
}
