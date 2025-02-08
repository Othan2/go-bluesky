package bluesky

import (
	"errors"
	"reflect"
	"strings"
)

// reflect all fields from request into a map. Used for getting param maps to send in xRPC requests.
func getParamMap(request any) (map[string]interface{}, error) {
	params := make(map[string]interface{})

	v := reflect.ValueOf(request)

	// Handle pointer input
	if v.Kind() == reflect.Ptr {
		// If it's nil, return empty map
		if v.IsNil() {
			return params, nil
		}
		v = v.Elem()
	}

	// Ensure we're working with a struct
	if v.Kind() != reflect.Struct {
		return nil, errors.New("Tried to reflect fields from a non-struct type, returning error.")
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldName := strings.ToLower(t.Field(i).Name)

		if !field.IsZero() {
			params[fieldName] = field.Interface()
		}
	}

	return params, nil
}
