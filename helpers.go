package draupnir

import "encoding/json"

// jsonMarshal encodes v as JSON.
//
// A previous version hand-rolled a "fast path" for primitives that emitted
// invalid JSON for strings containing quotes, backslashes, or control
// characters. Correctness wins: defer to encoding/json.
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// jsonUnmarshal decodes JSON data into v.
func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
