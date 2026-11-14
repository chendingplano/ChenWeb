package docprocessing

import "testing"

func TestRefIdentityKeysIgnoreEdition(t *testing.T) {
	cite, ok := RefCitationIdentityKey("GB/T 19095-2019")
	if !ok {
		t.Fatal("citation GB/T 19095-2019 has no key")
	}
	own, ok := RefOwnIdentityKey("GB/T 19095-2008", "生活垃圾分类标志")
	if !ok || own != cite {
		t.Fatalf("own key %q (ok %t) differs from citation key %q", own, ok, cite)
	}
	// A record without doc_no is identified from its file name, as extract_references does.
	own, ok = RefOwnIdentityKey("", "强制风冷制冷剂冷凝器性能试验方法", "164176875_GB_T 46121-2025_强制风冷制冷剂冷凝器性能试验方法_mineru.txt")
	cite, _ = RefCitationIdentityKey("GB/T 46121")
	if !ok || own != cite {
		t.Fatalf("file-name key %q (ok %t) differs from citation key %q", own, ok, cite)
	}
	// Generic standard prefixes are read from a document number.
	own, ok = RefOwnIdentityKey("JJG(苏)263-2024")
	cite, _ = RefCitationIdentityKey("JJG(苏)263")
	if !ok || own != cite {
		t.Fatalf("generic key %q (ok %t) differs from citation key %q", own, ok, cite)
	}
}

func TestRefCitationIdentityKeyNeedsAnIdentifier(t *testing.T) {
	if k, ok := RefCitationIdentityKey("《中华人民共和国固体废物污染环境防治法》"); ok {
		t.Fatalf("a title-only citation got key %q", k)
	}
}
