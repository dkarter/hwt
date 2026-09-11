package metadatajson

import (
	"reflect"
	"testing"
)

func TestDecodeObject(t *testing.T) {
	values, err := DecodeObject([]byte(`{"identifier":"RMS-85","empty":""}`))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(values, map[string]string{"identifier": "RMS-85", "empty": ""}) {
		t.Fatalf("unexpected values: %#v", values)
	}
}

func TestDecodeObjectRejectsNonStrings(t *testing.T) {
	for _, input := range []string{`null`, `[]`, `{"key":null}`, `{"key":42}`, `{"key":{}}`} {
		if _, err := DecodeObject([]byte(input)); err == nil {
			t.Fatalf("expected structured string-object error for %s, got %v", input, err)
		}
	}
}
