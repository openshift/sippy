package api

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/util/sets"
)

// FieldSelection contains the top-level JSON field names requested by a client.
type FieldSelection struct {
	sets.Set[string]
}

// ParseFieldSelection validates comma-separated top-level JSON field names against model.
// Empty input preserves the default full response.
func ParseFieldSelection(raw string, model any) (FieldSelection, error) {
	selection := FieldSelection{Set: sets.New[string]()}
	if strings.TrimSpace(raw) == "" {
		return selection, nil
	}
	allowed := sets.New[string]()
	t := reflect.TypeOf(model)
	for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array) {
		t = t.Elem()
	}
	if t != nil && t.Kind() == reflect.Struct {
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" {
				name = field.Name
			}
			if field.IsExported() && name != "-" {
				allowed.Insert(name)
			}
		}
	}
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if !allowed.Has(name) {
			return selection, &ValidationError{Message: fmt.Sprintf("invalid fields name: %q", name)}
		}
		selection.Insert(name)
	}
	return selection, nil
}

// Project retains selected fields in an object or array while preserving JSON encodings,
// including custom marshalers, omitted fields, null values, and integer precision.
func (s FieldSelection) Project(data any) (any, error) {
	if s.Len() == 0 {
		return data, nil
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("encoding fields response: %w", err)
	}
	return s.projectJSON(encoded)
}

// projectJSON applies the same selection to each array element or to an object's keys.
func (s FieldSelection) projectJSON(encoded json.RawMessage) (any, error) {
	if string(encoded) == "null" {
		return nil, nil
	}
	if len(encoded) > 0 && encoded[0] == '[' {
		var items []json.RawMessage
		if err := json.Unmarshal(encoded, &items); err != nil {
			return nil, fmt.Errorf("decoding fields array: %w", err)
		}
		result := make([]any, 0, len(items))
		for _, item := range items {
			projected, err := s.projectJSON(item)
			if err != nil {
				return nil, err
			}
			result = append(result, projected)
		}
		return result, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		return nil, fmt.Errorf("decoding fields object: %w", err)
	}
	for name := range object {
		if !s.Has(name) {
			delete(object, name)
		}
	}
	return object, nil
}
