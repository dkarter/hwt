package schema

import (
	"encoding/json"
	"testing"
)

func TestEmbeddedSchemaIsValidJSON(t *testing.T) {
	var document map[string]any
	if err := json.Unmarshal(JSON, &document); err != nil {
		t.Fatal(err)
	}
	if document["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("unexpected schema draft: %v", document["$schema"])
	}
}
