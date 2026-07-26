package model_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/cdm/cdmfixtures"
	"github.com/chendingplano/deepdoc/server/api/cdm/model"
)

func TestJSONRoundTrip(t *testing.T) {
	docs := map[string]model.Document{
		"jaro-winkler":    cdmfixtures.JaroWinkler(),
		"all-block-types": cdmfixtures.AllBlockTypes(),
	}

	for name, doc := range docs {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(doc)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			var got model.Document
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if !reflect.DeepEqual(doc, got) {
				t.Errorf("round trip mismatch:\nwant: %+v\ngot:  %+v", doc, got)
			}
		})
	}
}
