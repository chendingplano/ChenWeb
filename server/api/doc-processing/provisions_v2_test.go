package docprocessing

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

const provisionV2Example = `{"v":2,"l":"zh","p":[{"n":"垃圾收运","t":"m","x":"垃圾应每日收运。","d":"每日收运要求","s":[42,"45-46"],"c":"收运要求","u":"垃圾","lt":"b","k":["收运"],"q":0.95,"f":1,"o":[{"n":"垃圾","t":"mat","r":"reg","s":[42],"q":0.9,"d":"分类后的垃圾","a":["生活垃圾"]}]}]}`

func TestProvisionsV2AcceptsEmptyExtraction(t *testing.T) {
	got := extractProvisionTestPayload(t, `{"v":2,"l":"zh","p":[]}`)
	if len(got["provisions"].([]any)) != 0 {
		t.Fatalf("expected successful empty extraction, got %#v", got)
	}
}

func TestProvisionsV2DecodesSourceOnlyFields(t *testing.T) {
	got := extractProvisionTestPayload(t, provisionV2Example)
	p := got["provisions"].([]any)[0].(map[string]any)
	for key, want := range map[string]any{
		"name": "垃圾收运", "type": "mandatory", "provision": "垃圾应每日收运。", "provision_desc": "每日收运要求",
		"source_line_spans": []string{"42", "45-46"}, "context": "收运要求", "subject": "垃圾", "location_type": "bullet",
		"keywords": []string{"收运"}, "confidence": 0.95, "is_explicit": true, "need_verify": false,
	} {
		if !reflect.DeepEqual(p[key], want) {
			t.Errorf("%s=%#v, want %#v", key, p[key], want)
		}
	}
	for _, key := range []string{"name_en", "provision_en", "subject_en", "context_en", "provision_desc_en"} {
		if asString(p[key]) != "" {
			t.Errorf("generated translation in %s: %#v", key, p[key])
		}
	}
	o := objectItemsFromValue(p["objects"])[0]
	if o["object_type"] != "material" || o["object_role"] != "regulated_object" || asString(o["evidence_quote"]) != "" || asString(o["object_name_en"]) != "" {
		t.Fatalf("incorrect object decoding: %#v", o)
	}
	if !reflect.DeepEqual(o["source_line_spans"], []string{"42"}) || !reflect.DeepEqual(o["acronyms"], []string{}) {
		t.Fatalf("object spans or empty defaults incorrect: %#v", o)
	}
}

func TestProvisionsV2Run258SourceOnlyReplay(t *testing.T) {
	f, err := os.Open("testdata/provisions-run-258.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	var fixture struct {
		Responses []map[string]any `json:"responses"`
	}
	if err := json.NewDecoder(z).Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	var baselineChars, wireChars, baselineBytes, wireBytes, count, objects int
	processor := &ProvisionsProcessor{PromptRef: "replay"}
	for _, full := range fixture.Responses {
		baseline, _ := json.Marshal(full)
		wire, expected := encodeProvisionV2Replay(full)
		encoded, _ := json.Marshal(wire)
		baselineChars += utf8.RuneCount(baseline)
		wireChars += utf8.RuneCount(encoded)
		baselineBytes += len(baseline)
		wireBytes += len(encoded)
		decoded := extractProvisionTestPayload(t, string(encoded))
		want := processor.buildProvisionOutputRows(416, normalizeProvisionList(expected, nil, nil), time.Unix(0, 0), 10, 0, "replay")
		got := processor.buildProvisionOutputRows(416, normalizeProvisionList(decoded["provisions"].([]any), nil, nil), time.Unix(0, 0), 10, 0, "replay")
		count += len(got)
		for i := range want {
			for name, pair := range map[string][2]any{
				"storage":  {buildProvisionDBRecord(want[i], "zh"), buildProvisionDBRecord(got[i], "zh")},
				"artifact": {buildProvisionFileRecord(want[i]), buildProvisionFileRecord(got[i])},
				"objects":  {normalizeArtifactObjectsForArtifact(416, searchArtifactProvision, "replay", want[i]), normalizeArtifactObjectsForArtifact(416, searchArtifactProvision, "replay", got[i])},
				"search":   {buildProvisionSearchDocument(want[i], true), buildProvisionSearchDocument(got[i], true)},
			} {
				a, _ := json.Marshal(pair[0])
				b, _ := json.Marshal(pair[1])
				if string(a) != string(b) {
					t.Fatalf("source-only %s changed for provision %d:\nwant %s\ngot %s", name, i, a, b)
				}
			}
			objects += len(objectItemsFromValue(got[i]["objects"]))
		}
	}
	if count != 64 || objects != 152 || wireChars >= baselineChars/3 {
		t.Fatalf("count=%d objects=%d chars=%d/%d", count, objects, wireChars, baselineChars)
	}
	t.Logf("%d provisions, %d objects: %d -> %d characters (%.2f%% smaller), %d -> %d bytes (%.2f%% smaller); source-only outputs preserved", count, objects, baselineChars, wireChars, 100*(1-float64(wireChars)/float64(baselineChars)), baselineBytes, wireBytes, 100*(1-float64(wireBytes)/float64(baselineBytes)))
}

// Test-only encoder for the archived source-language values. Production asks the
// LLM for this wire format directly; it does not generate an oversized response
// and then compress it after paying for the tokens.
func encodeProvisionV2Replay(full map[string]any) (map[string]any, []any) {
	clone, _ := json.Marshal(full)
	var source map[string]any
	_ = json.Unmarshal(clone, &source)
	items := source["provisions"].([]any)
	wireItems := make([]any, 0, len(items))
	pkeys := map[string]string{"name": "n", "type": "t", "provision": "x", "provision_desc": "d", "source_line_spans": "s", "context": "c", "subject": "u", "location_type": "lt", "keywords": "k", "confidence": "q", "objects": "o", "category_paths": "g"}
	okeys := map[string]string{"object_name": "n", "object_type": "t", "object_role": "r", "source_line_spans": "s", "confidence": "q", "description": "d", "aliases": "a", "acronyms": "y"}
	for _, item := range items {
		p := item.(map[string]any)
		for key := range p {
			if strings.HasSuffix(key, "_en") {
				delete(p, key)
			}
		}
		wire := map[string]any{"f": 0}
		if p["is_explicit"] == true {
			wire["f"] = wire["f"].(int) | 1
		}
		if p["need_verify"] == true {
			wire["f"] = wire["f"].(int) | 2
		}
		for key, short := range pkeys {
			if v, ok := p[key]; ok {
				wire[short] = v
			}
		}
		wire["t"] = reverseProvisionV2Enum(provisionV2Types, asString(p["type"]))
		wire["lt"] = reverseProvisionV2Enum(provisionV2Locations, asString(p["location_type"]))
		wire["s"] = numericProvisionV2ReplaySpans(p["source_line_spans"])
		wireObjects := []any{}
		for _, object := range objectItemsFromValue(p["objects"]) {
			delete(object, "object_name_en")
			delete(object, "evidence_quote")
			delete(object, "object_name_zh")
			object["object_name_zh"] = object["object_name"] // native Chinese, not translation
			o := map[string]any{}
			for key, short := range okeys {
				if v, ok := object[key]; ok {
					o[short] = v
				}
			}
			o["t"] = reverseProvisionV2Enum(provisionV2ObjectTypes, asString(object["object_type"]))
			o["r"] = reverseProvisionV2Enum(provisionV2ObjectRoles, asString(object["object_role"]))
			o["s"] = numericProvisionV2ReplaySpans(object["source_line_spans"])
			omitEmptyProvisionV2ReplayFields(o)
			// Expanded object defaults match legacy serialized storage.
			if _, ok := object["description"]; !ok {
				object["description"] = ""
			}
			wireObjects = append(wireObjects, o)
		}
		wire["o"] = wireObjects
		omitEmptyProvisionV2ReplayFields(wire)
		wireItems = append(wireItems, wire)
	}
	return map[string]any{"v": 2, "l": source["language"], "p": wireItems}, items
}

func reverseProvisionV2Enum(values map[string]string, value string) string {
	for code, expanded := range values {
		if expanded == value {
			return code
		}
	}
	return value
}

func numericProvisionV2ReplaySpans(value any) []any {
	result := []any{}
	for _, span := range toStringSlice(value) {
		if n, err := strconv.Atoi(span); err == nil {
			result = append(result, n)
		} else {
			result = append(result, span)
		}
	}
	return result
}

func omitEmptyProvisionV2ReplayFields(record map[string]any) {
	for key, value := range record {
		if key == "g" {
			continue
		}
		if text, ok := value.(string); ok && text == "" {
			delete(record, key)
		}
		if list, ok := value.([]any); ok && len(list) == 0 {
			delete(record, key)
		}
	}
}

func TestProvisionsV2PreservesAllPackedFlags(t *testing.T) {
	for flags := 0; flags < 4; flags++ {
		var raw map[string]any
		if err := json.Unmarshal([]byte(provisionV2Example), &raw); err != nil {
			t.Fatal(err)
		}
		raw["p"].([]any)[0].(map[string]any)["f"] = flags
		wire, _ := json.Marshal(raw)
		got := extractProvisionTestPayload(t, string(wire))["provisions"].([]any)[0].(map[string]any)
		if got["is_explicit"] != (flags&1 != 0) || got["need_verify"] != (flags&2 != 0) {
			t.Fatalf("f=%d decoded incorrectly: %#v", flags, got)
		}
	}
}

func TestProvisionsV2RejectsInvalidWireRecords(t *testing.T) {
	for _, mutate := range []func(map[string]any){
		func(p map[string]any) { delete(p, "s") },
		func(p map[string]any) { p["s"] = []any{} },
		func(p map[string]any) { p["s"] = []any{42, "invalid"} },
		func(p map[string]any) { p["s"] = []any{"46-45"} },
		func(p map[string]any) { p["s"] = []any{42.5} },
		func(p map[string]any) { p["s"] = nil },
		func(p map[string]any) { delete(p, "f") },
		func(p map[string]any) { p["f"] = 4 },
		func(p map[string]any) { p["f"] = "1" },
		func(p map[string]any) { p["t"] = "unknown" },
		func(p map[string]any) { p["q"] = -0.1 },
		func(p map[string]any) { p["name_en"] = "translation" },
		func(p map[string]any) { p["o"].([]any)[0].(map[string]any)["s"] = []any{} },
		func(p map[string]any) { delete(p["o"].([]any)[0].(map[string]any), "s") },
		func(p map[string]any) { p["o"].([]any)[0].(map[string]any)["eq"] = "quote" },
		func(p map[string]any) { p["o"] = nil },
	} {
		var raw map[string]any
		if err := json.Unmarshal([]byte(provisionV2Example), &raw); err != nil {
			t.Fatal(err)
		}
		mutate(raw["p"].([]any)[0].(map[string]any))
		processor := &ProvisionsProcessor{PromptRef: "test"}
		got, err := processor.extractProvisionPayloadFromText(context.Background(), "input", "instructions", "test", &fakeJSONExtractor{out: raw})
		if err == nil || got != nil {
			t.Fatalf("invalid short response accepted: %#v, err=%v", raw, err)
		}
	}
}

func TestProvisionsV2KeepsEnglishSourceInStorage(t *testing.T) {
	got := extractProvisionTestPayload(t, `{"v":2,"l":"en","p":[{"n":"Inspection","t":"m","x":"The operator shall inspect daily.","s":[42],"lt":"s","q":0.9,"f":1}]}`)
	items := normalizeProvisionList(got["provisions"].([]any), nil, nil)
	rows := (&ProvisionsProcessor{}).buildProvisionOutputRows(1, items, time.Unix(0, 0), 1, 0, "test")
	record := buildProvisionDBRecord(rows[0], "en")
	if record["provision_en"] != "The operator shall inspect daily." || record["source_text"] != "The operator shall inspect daily." {
		t.Fatalf("English source text lost: %#v", record)
	}
}

func TestProvisionsV2DoesNotMutateCapturedResponse(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(provisionV2Example), &raw); err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(raw)
	p := &ProvisionsProcessor{PromptRef: "test"}
	if _, err := p.extractProvisionPayloadFromText(context.Background(), "input", "instructions", "test", &fakeJSONExtractor{out: raw}); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(raw)
	if string(before) != string(after) || strings.Contains(string(after), "source_line_spans") {
		t.Fatal("captured wire response was expanded in place")
	}
}

func TestProvisionsV2StructuredRetriesNonconformingResponses(t *testing.T) {
	for _, first := range []string{
		`{"provisions":[{"provision":"legacy translated response","source_line_spans":[42]}]}`,
		strings.Replace(provisionV2Example, `"s":[42]`, `"s":[]`, 1),
		strings.Replace(provisionV2Example, `"f":1`, `"f":4`, 1),
	} {
		var requests atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			source := provisionV2Example
			if requests.Add(1) == 1 {
				source = first
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": source}}}})
		}))
		client := &llmclients.OpenAIJSONClient{BaseURL: srv.URL, APIKey: "test-key", ModelName: "test", HTTPClient: srv.Client()}
		p := &ProvisionsProcessor{PromptRef: "prompt-extract-provisions-v6.md"}
		got, err := p.extractProvisionPayloadFromText(context.Background(), "input", "instructions", "test", client)
		srv.Close()
		if err != nil || got == nil || requests.Load() != 2 {
			t.Fatalf("requests=%d got=%#v err=%v; v6 must retry incompatible output", requests.Load(), got, err)
		}
	}
}
