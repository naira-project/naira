package pgstore

import (
	"encoding/json"
	"fmt"
)

func encodeProperties(properties map[string]string) ([]byte, error) {
	if properties == nil {
		properties = map[string]string{}
	}
	raw, err := json.Marshal(properties)
	if err != nil {
		return nil, fmt.Errorf("marshaling properties: %w", err)
	}
	return raw, nil
}

func decodeProperties(raw []byte) (map[string]string, error) {
	var properties map[string]string
	if err := json.Unmarshal(raw, &properties); err != nil {
		return nil, fmt.Errorf("unmarshaling properties: %w", err)
	}
	return properties, nil
}
