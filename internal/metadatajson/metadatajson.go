package metadatajson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

func DecodeObject(data []byte) (map[string]string, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, errors.New("must be a JSON object with string values")
	}
	values := make(map[string]string, len(raw))
	for key, encoded := range raw {
		if bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
			return nil, fmt.Errorf("field %q must be a string, got null", key)
		}
		var value string
		if err := json.Unmarshal(encoded, &value); err != nil {
			return nil, fmt.Errorf("field %q must be a string: %w", key, err)
		}
		values[key] = value
	}
	return values, nil
}
