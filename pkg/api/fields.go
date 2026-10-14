package api

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// FieldSelection describes requested JSON fields. A nil subtree selects the entire field.
type FieldSelection map[string]FieldSelection

// ParseFieldSelection validates comma-separated JSON field paths against model.
// Empty input preserves the default full response. Nested structs and slices support dotted paths.
func ParseFieldSelection(raw string, model any) (FieldSelection, error) {
	selection := FieldSelection{}
	if strings.TrimSpace(raw) == "" {
		return selection, nil
	}
	for _, path := range strings.Split(raw, ",") {
		path = strings.TrimSpace(path)
		parts := strings.Split(path, ".")
		t := reflect.TypeOf(model)
		for _, part := range parts {
			for t != nil && (t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array) {
				t = t.Elem()
			}
			var fieldType reflect.Type
			if t != nil && t.Kind() == reflect.Struct {
				for i := 0; i < t.NumField(); i++ {
					field := t.Field(i)
					name := strings.Split(field.Tag.Get("json"), ",")[0]
					if name == "" {
						name = field.Name
					}
					if field.IsExported() && name != "-" && name == part {
						fieldType = field.Type
						break
					}
				}
			}
			if fieldType == nil {
				return nil, &ValidationError{Message: fmt.Sprintf("invalid fields path: %q", path)}
			}
			t = fieldType
		}
		selection.add(parts)
	}
	return selection, nil
}

// add merges paths so selecting a parent always includes all of its children.
func (s FieldSelection) add(parts []string) {
	name := parts[0]
	if len(parts) == 1 {
		s[name] = nil
		return
	}
	child, exists := s[name]
	if exists && child == nil {
		return
	}
	if !exists {
		child = FieldSelection{}
		s[name] = child
	}
	child.add(parts[1:])
}

// Project retains selected fields in an object or array while preserving JSON encodings,
// including custom marshalers, omitted fields, null values, and integer precision.
func (s FieldSelection) Project(data any) (any, error) {
	if len(s) == 0 {
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
	result := make(map[string]any, len(s))
	for name, child := range s {
		value, exists := object[name]
		if !exists {
			continue
		}
		if child == nil {
			result[name] = value
			continue
		}
		projected, err := child.projectJSON(value)
		if err != nil {
			return nil, err
		}
		result[name] = projected
	}
	return result, nil
}
