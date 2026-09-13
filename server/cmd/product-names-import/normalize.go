package main

import (
	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
)

// keySetJSON is the JSON-friendly shape of one semid.KeySet, storable in
// kb.product_names.keywords. It intentionally never touches
// kb.keyword_surfaces / kb.keyword_concepts — this is normalization only,
// not concept resolution (that integration is deferred).
type keySetJSON struct {
	Norm     string `json:"norm"`
	Alnum    string `json:"alnum,omitempty"`
	Sorted   string `json:"sorted,omitempty"`
	Singular string `json:"singular,omitempty"`
	Initials string `json:"initials,omitempty"`
}

// productNameKeywords is the kb.product_names.keywords JSONB shape: the
// shared normalizer's derived keys for the Chinese name and, when a
// translation exists, the English name.
type productNameKeywords struct {
	Zh keySetJSON  `json:"zh"`
	En *keySetJSON `json:"en,omitempty"`
}

var normalizer = semid.Normalizer{Version: semid.CurrentNormalizerVersion}

func toKeySetJSON(ks semid.KeySet) keySetJSON {
	return keySetJSON{
		Norm:     ks.Norm,
		Alnum:    ks.Alnum,
		Sorted:   ks.Sorted,
		Singular: ks.Singular,
		Initials: ks.Initials,
	}
}

// buildKeywords normalizes name (and nameEN, if non-empty) via the shared
// semid.Normalizer, the same pipeline tiers 0-4 of the keyword-canonicalization
// ladder read (spec doc-2026080403 §6, §9.1). It never calls the resolver or
// writes to the keyword-module's own tables.
func buildKeywords(name, nameEN string) productNameKeywords {
	out := productNameKeywords{Zh: toKeySetJSON(normalizer.Normalize(name))}
	if nameEN != "" {
		en := toKeySetJSON(normalizer.Normalize(nameEN))
		out.En = &en
	}
	return out
}
