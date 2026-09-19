package sdk

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
)

// JSONValue is any JSON-compatible value, including null in nested positions.
// Maps must have string keys. JSONContent is the subset accepted for state and
// question instructions: a string, object, or array.
type JSONValue = any
type JSONContent = any

func validateJSON(v any, path string, topLevelContent bool) error {
	if v == nil {
		if topLevelContent {
			return fmt.Errorf("%s must be a string, object, or array", displayPath(path))
		}
		return nil
	}

	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return nil
	}
	for rv.Kind() == reflect.Interface || rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			if topLevelContent {
				return fmt.Errorf("%s must be a string, object, or array", displayPath(path))
			}
			return nil
		}
		rv = rv.Elem()
	}

	if topLevelContent && rv.Kind() != reflect.String && rv.Kind() != reflect.Map && rv.Kind() != reflect.Array && rv.Kind() != reflect.Slice {
		return fmt.Errorf("%s must be a string, object, or array", displayPath(path))
	}

	switch rv.Kind() {
	case reflect.String, reflect.Bool:
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return nil
	case reflect.Float32, reflect.Float64:
		if math.IsNaN(rv.Float()) || math.IsInf(rv.Float(), 0) {
			return fmt.Errorf("%s contains a non-finite number", displayPath(path))
		}
		return nil
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("%s has a non-string map key", displayPath(path))
		}
		iter := rv.MapRange()
		for iter.Next() {
			key := iter.Key().String()
			if err := validateJSON(iter.Value().Interface(), joinPath(path, key), false); err != nil {
				return err
			}
		}
		return nil
	case reflect.Array, reflect.Slice:
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			if topLevelContent {
				return nil
			}
			return nil
		}
		for i := 0; i < rv.Len(); i++ {
			if err := validateJSON(rv.Index(i).Interface(), fmt.Sprintf("%s[%d]", path, i), false); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		// encoding/json supports structs, but request content is intentionally
		// represented by JSON-like values rather than arbitrary Go objects.
		if _, ok := v.(json.Marshaler); ok {
			return nil
		}
		return fmt.Errorf("%s contains unsupported value of type %s", displayPath(path), rv.Type())
	default:
		return fmt.Errorf("%s contains unsupported value of type %s", displayPath(path), rv.Type())
	}
}

func validateContent(v any, path string) error {
	return validateJSON(v, path, true)
}

func marshalJSON(v any) ([]byte, error) {
	if err := validateJSON(v, "value", false); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func displayPath(path string) string {
	if path == "" {
		return "value"
	}
	return path
}

func joinPath(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func numberString(v any) string {
	switch n := v.(type) {
	case float32:
		return strconv.FormatFloat(float64(n), 'g', -1, 32)
	case float64:
		return strconv.FormatFloat(n, 'g', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}
