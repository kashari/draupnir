package draupnir

import (
	"encoding/json"
	"reflect"
	"strconv"
)

// jsonMarshal is a faster version of json.Marshal
func jsonMarshal(v any) ([]byte, error) {
	// Use reflection to check if v is a primitive type
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String:
		return []byte(`"` + v.(string) + `"`), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return []byte(strconv.FormatInt(rv.Int(), 10)), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return []byte(strconv.FormatUint(rv.Uint(), 10)), nil
	case reflect.Float32, reflect.Float64:
		return []byte(strconv.FormatFloat(rv.Float(), 'f', -1, 64)), nil
	case reflect.Bool:
		if rv.Bool() {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case reflect.Ptr, reflect.Interface:
		if rv.IsNil() {
			return []byte("null"), nil
		}
		return jsonMarshal(rv.Elem().Interface())
	}

	// Use standard json.Marshal for complex types
	return json.Marshal(v)
}

// jsonUnmarshal is a faster version of json.Unmarshal
func jsonUnmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}
