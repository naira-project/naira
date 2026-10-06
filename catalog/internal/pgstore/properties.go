package pgstore

import "encoding/json"

func encodeProperties(properties map[string]string) ([]byte, error) {
	if properties == nil {
		properties = map[string]string{}
	}
	return json.Marshal(properties)
}

func decodeProperties(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return map[string]string{}, nil
	}
	var properties map[string]string
	if err := json.Unmarshal(raw, &properties); err != nil {
		return nil, err
	}
	return properties, nil
}
