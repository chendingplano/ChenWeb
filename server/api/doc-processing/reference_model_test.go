package docprocessing

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestParseRefIdentifiers(t *testing.T) {
	cases := []struct {
		raw  string
		want []RefIdentifier
	}{
		{"GB/T755-2019", []RefIdentifier{{Kind: "standard", Value: "GB/T 755", Edition: "2019"}}},
		{"按照GB/T755确定", []RefIdentifier{{Kind: "standard", Value: "GB/T 755"}}},
		{"ISO 14253-1", []RefIdentifier{{Kind: "standard", Value: "ISO 14253-1"}}},
		{"ISO 13485:2016, Clause 7.5", []RefIdentifier{{Kind: "standard", Value: "ISO 13485", Edition: "2016"}}},
		{"GB 7959—2012 粪便无害化卫生要求", []RefIdentifier{{Kind: "standard", Value: "GB 7959", Edition: "2012"}}},
		{"IEC60529", []RefIdentifier{{Kind: "standard", Value: "IEC 60529"}}},
		{"T/CI 140—2023", []RefIdentifier{{Kind: "standard", Value: "T/CI 140", Edition: "2023"}}},
		{"RFC 9110", []RefIdentifier{{Kind: "rfc", Value: "RFC 9110"}}},
		{"doi:10.1145/1234567.", []RefIdentifier{{Kind: "doi", Value: "10.1145/1234567"}}},
		{"arXiv:2609.15364v2", []RefIdentifier{{Kind: "arxiv", Value: "2609.15364", Edition: "2"}}},
		{"Regulation (EU) 2016/679", []RefIdentifier{{Kind: "legal", Value: "Regulation (EU) 2016/679"}}},
		{"21 CFR Part 820", []RefIdentifier{{Kind: "legal", Value: "21 CFR 820"}}},
		{"see https://example.org/ISO-9001.pdf", []RefIdentifier{{Kind: "url", Value: "https://example.org/ISO-9001.pdf"}}},
		{"the previous report", nil},
	}
	for _, c := range cases {
		got := parseRefIdentifiers(c.raw)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseRefIdentifiers(%q) = %+v, want %+v", c.raw, got, c.want)
		}
	}
}

func TestParseRefIdentifiersForMention_GenericStandard(t *testing.T) {
	got := parseRefIdentifiersForMention("standard_number", "CJJ 27-2012")
	want := []RefIdentifier{{Kind: "standard", Value: "CJJ 27", Edition: "2012"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if got := parseRefIdentifiersForMention("named_reference", "CJJ 27-2012"); len(got) != 0 {
		t.Fatalf("generic prefix used without standard_number mechanism: %+v", got)
	}
}

func TestRefIdentityKey(t *testing.T) {
	key, ed, public, ok := refIdentityKey([]RefIdentifier{{Kind: "url", Value: "https://x"}, {Kind: "standard", Value: "GB/T 755", Edition: "2019"}}, "")
	if key != "standard:GB/T 755" || ed != "2019" || !public || !ok {
		t.Fatalf("standard should win: %q %q %v %v", key, ed, public, ok)
	}
	key, _, public, ok = refIdentityKey(nil, "  Company  Policy ")
	if key != "title:company policy" || public || !ok {
		t.Fatalf("title key must be private: %q %v %v", key, public, ok)
	}
	if _, _, _, ok := refIdentityKey(nil, ""); ok {
		t.Fatal("nothing named must not produce a key")
	}
}

func TestRefOwnIdentifier_FromFilename(t *testing.T) {
	id, ok := refOwnIdentifier(2, "", "", "强制风冷制冷剂冷凝器性能试验方法", "164176875_GB_T 46121-2025_强制风冷制冷剂冷凝器性能试验方法_mineru.txt")
	if !ok || id.Value != "GB/T 46121" || id.Edition != "2025" {
		t.Fatalf("got %+v %v", id, ok)
	}
	id, ok = refOwnIdentifier(1, "JJG(苏)263-2024", "光柱式血压计检定规程")
	if !ok || id.Value != "JJG(苏) 263" || id.Edition != "2024" {
		t.Fatalf("regional standard number from doc_no: got %+v %v", id, ok)
	}
}

func TestNormalizeRefMention(t *testing.T) {
	m := RefMention{TargetType: "Standard", TargetSubtype: "fancy_standard", Mechanism: "carrier_pigeon",
		Relationships: []string{"requires", "contradicts", "requires"}, TargetScope: "section", TargetContent: "criterion",
		RefForm: "listed", ListKind: "something"}
	normalizeRefMention(&m)
	if m.TargetType != "standard" || m.TargetSubtype != "others" || m.TargetSubtypeLabel != "fancy_standard" {
		t.Fatalf("subtype: %+v", m)
	}
	if m.Mechanism != "named_reference" {
		t.Fatalf("mechanism: %q", m.Mechanism)
	}
	if !reflect.DeepEqual(m.Relationships, []string{"requires"}) {
		t.Fatalf("derived relationship must be dropped and duplicates removed: %v", m.Relationships)
	}
	if m.ListKind != refListInformative {
		t.Fatalf("unknown list kind must default to informative: %q", m.ListKind)
	}
	s := RefMention{Mechanism: "structural_reference", TargetContent: "general"}
	normalizeRefMention(&s)
	if !s.Internal || s.RefForm != refFormInText || !reflect.DeepEqual(s.Relationships, []string{"references"}) {
		t.Fatalf("structural reference: %+v", s)
	}
}

func refTestMention(form, listKind, mech, raw string, inReq bool) RefMention {
	m := RefMention{RefForm: form, ListKind: listKind, Mechanism: mech, CitedRaw: raw, InRequirement: inReq, TargetContent: "general"}
	normalizeRefMention(&m)
	m.Identifiers = parseRefIdentifiersForMention(m.Mechanism, raw)
	return m
}

func TestDeriveRefNormativity(t *testing.T) {
	ms := []RefMention{
		refTestMention("listed", "normative", "standard_number", "GB/T 755 旋转电机 定额和性能", false), // 0
		refTestMention("listed", "normative", "standard_number", "GB/T 1236", false),           // 1 never cited
		refTestMention("listed", "informative", "standard_number", "ISO 14253-1", false),       // 2
		refTestMention("in_text", "", "standard_number", "GB/T755-2019", true),                 // 3 normative
		refTestMention("in_text", "", "standard_number", "GB 7959", true),                      // 4 not listed
		refTestMention("in_text", "", "standard_number", "ISO 14253-1", false),                 // 5 informative
	}
	deriveRefNormativity(ms)
	want := []struct {
		norm, mismatch string
		entry          int
	}{
		{"normative", "", -1}, {"normative", "never_cited", -1}, {"informative", "", -1},
		{"normative", "", 0}, {"unlisted", "not_listed", -1}, {"informative", "", 2},
	}
	for i, w := range want {
		if ms[i].Normativity != w.norm || ms[i].ListMismatch != w.mismatch || ms[i].ListEntryIdx != w.entry {
			t.Errorf("mention %d: got (%s, %q, %d), want (%s, %q, %d)", i, ms[i].Normativity, ms[i].ListMismatch, ms[i].ListEntryIdx, w.norm, w.mismatch, w.entry)
		}
	}
}

func TestDeriveRefNormativity_NoListMeansUnlistedWithoutMismatch(t *testing.T) {
	ms := []RefMention{refTestMention("in_text", "", "legal_citation", "shall comply with Regulation (EU) 2016/679", true)}
	deriveRefNormativity(ms)
	if ms[0].Normativity != "unlisted" || ms[0].ListMismatch != "" || !ms[0].InRequirement {
		t.Fatalf("got %+v", ms[0])
	}
}

func TestDeriveRefNormativity_LabelCitationInheritsEntry(t *testing.T) {
	entry := refTestMention("listed", "informative", "doi", "Smith J. et al. Fan noise. 2024. doi:10.1000/abc", false)
	entry.ListLabel = "[12]"
	cite := refTestMention("in_text", "", "numbered_citation", "[12]", false)
	ms := []RefMention{entry, cite}
	deriveRefNormativity(ms)
	if ms[1].ListEntryIdx != 0 || ms[1].Normativity != "informative" || len(ms[1].Identifiers) != 1 || ms[1].Identifiers[0].Value != "10.1000/abc" {
		t.Fatalf("got %+v", ms[1])
	}
}

func TestRefMentionKeyStable(t *testing.T) {
	a := RefMention{InputRecordID: 1, Section: "5 要求", SourceLineSpans: []string{"120"}, CitedRaw: "GB  7959", MetricName: "粪大肠菌群数"}
	b := a
	b.CitedRaw = "gb 7959"
	if refMentionKey(a) != refMentionKey(b) {
		t.Fatal("key must ignore case and spacing")
	}
	b.MetricName = "蛔虫卵死亡率"
	if refMentionKey(a) == refMentionKey(b) {
		t.Fatal("different quantities must differ")
	}
}

func TestNewReferencesProcessor_NoReasoningPolicy(t *testing.T) {
	modelsPath := filepath.Join(t.TempDir(), ".models.toml")
	body := "[deepseek-flash-chen]\nhost = \"cloud\"\nmodel_name = \"deepseek-v4-flash\"\napi_key = \"sk-test\"\nbase_url = \"https://api.deepseek.com\"\nthinking_type = \"enabled\"\n"
	if err := os.WriteFile(modelsPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MODEL_DEF_FILE", modelsPath)
	t.Setenv("EXTRACT_REFERENCES_MODEL_NAME", "deepseek-flash-chen")

	t.Setenv("MODEL_DEFAULT_REASONING_POLICY", "")
	kept := &llmclients.OpenAIJSONClient{}
	NewReferencesProcessor(nil, nil, kept, nil)
	if kept.ThinkingType != "enabled" {
		t.Fatalf("without the policy the model's thinking config is kept, got %q", kept.ThinkingType)
	}

	t.Setenv("MODEL_DEFAULT_REASONING_POLICY", "no-reasoning")
	off := &llmclients.OpenAIJSONClient{}
	NewReferencesProcessor(nil, nil, off, nil)
	if off.ThinkingType != "disabled" {
		t.Fatalf("no-reasoning policy must disable thinking, got %q", off.ThinkingType)
	}
}
