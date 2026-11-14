package docprocessing

// Identity keys for code outside extract_references. The compliance gold pool uses them to find
// the knowledge-base records that hold a document an activity cites. A key has no edition, so every
// edition of a cited standard has the same key ("GB/T 19095-2019" and "GB/T 19095-2008").

// RefCitationIdentityKey returns the identity key of a citation text such as "GB/T 19095-2019":
// "<kind>:<value>" of its highest-priority identifier. ok is false when the text holds no
// identifier (a citation by title alone).
func RefCitationIdentityKey(citation string) (key string, ok bool) {
	key, _, _, ok = refIdentityKey(parseRefIdentifiersForMention("standard_number", citation), "")
	return key, ok
}

// RefOwnIdentityKey returns the identity key of a document's own identifier, read as
// extract_references attaches documents: the document number first (where a generic standard
// pattern is allowed), then the title and file names in the order given.
func RefOwnIdentityKey(docNo string, names ...string) (string, bool) {
	id, ok := refOwnIdentifier(1, append([]string{docNo}, names...)...)
	if !ok {
		return "", false
	}
	return id.Kind + ":" + id.Value, true
}
