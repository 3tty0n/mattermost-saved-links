package kvstore

import "encoding/json"

// marshalValue mirrors pluginapi.KVService.Set: byte slices are stored as-is, other values are
// JSON encoded, and nil deletes the key.
func marshalValue(value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	if raw, ok := value.([]byte); ok {
		return raw, nil
	}
	return json.Marshal(value)
}

func unmarshalValue(raw []byte, out any) error {
	if target, ok := out.(*[]byte); ok {
		*target = append((*target)[:0], raw...)
		return nil
	}
	return json.Unmarshal(raw, out)
}
