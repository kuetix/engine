package workflow

import (
	"reflect"
	"testing"
)

func TestParseWSLValue_SimpleArrays(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
		wantOk   bool
	}{
		{
			name:     "empty array",
			input:    "[]",
			expected: []interface{}{},
			wantOk:   true,
		},
		{
			name:     "simple number array",
			input:    "[1, 2, 3]",
			expected: []interface{}{1, 2, 3},
			wantOk:   true,
		},
		{
			name:     "simple string array",
			input:    `["a", "b", "c"]`,
			expected: []interface{}{"a", "b", "c"},
			wantOk:   true,
		},
		{
			name:     "mixed types array",
			input:    `[1, "string", true, false]`,
			expected: []interface{}{1, "string", true, false},
			wantOk:   true,
		},
		{
			name:     "array with float",
			input:    "[1.5, 2.5, 3.5]",
			expected: []interface{}{1.5, 2.5, 3.5},
			wantOk:   true,
		},
		{
			name:     "array with spaces",
			input:    "[ 1 , 2 , 3 ]",
			expected: []interface{}{1, 2, 3},
			wantOk:   true,
		},
		{
			name:     "array with newlines",
			input:    "[\n  1,\n  2,\n  3\n]",
			expected: []interface{}{1, 2, 3},
			wantOk:   true,
		},
		{
			name:     "array with trailing comma",
			input:    "[1, 2, 3,]",
			expected: []interface{}{1, 2, 3},
			wantOk:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseWSLValue(tt.input)
			if ok != tt.wantOk {
				t.Errorf("parseWSLValue() ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseWSLValue() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseWSLValue_NestedArrays(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
		wantOk   bool
	}{
		{
			name:     "nested array 2 levels",
			input:    "[[1, 2], [3, 4]]",
			expected: []interface{}{[]interface{}{1, 2}, []interface{}{3, 4}},
			wantOk:   true,
		},
		{
			name:     "nested array 3 levels",
			input:    "[[[1, 2]], [[3, 4]]]",
			expected: []interface{}{[]interface{}{[]interface{}{1, 2}}, []interface{}{[]interface{}{3, 4}}},
			wantOk:   true,
		},
		{
			name:     "mixed nested array",
			input:    "[1, [2, 3], 4]",
			expected: []interface{}{1, []interface{}{2, 3}, 4},
			wantOk:   true,
		},
		{
			name:     "deeply nested with strings",
			input:    `[["a", "b"], ["c", "d"]]`,
			expected: []interface{}{[]interface{}{"a", "b"}, []interface{}{"c", "d"}},
			wantOk:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseWSLValue(tt.input)
			if ok != tt.wantOk {
				t.Errorf("parseWSLValue() ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseWSLValue() = %#v, want %#v", got, tt.expected)
			}
		})
	}
}

func TestParseWSLValue_ArraysWithObjects(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
		wantOk   bool
	}{
		{
			name:  "array of objects",
			input: `[{name: "Alice"}, {name: "Bob"}]`,
			expected: []interface{}{
				map[string]interface{}{"name": "Alice"},
				map[string]interface{}{"name": "Bob"},
			},
			wantOk: true,
		},
		{
			name:  "array of objects with multiple fields",
			input: `[{id: 1, name: "Alice"}, {id: 2, name: "Bob"}]`,
			expected: []interface{}{
				map[string]interface{}{"id": 1, "name": "Alice"},
				map[string]interface{}{"id": 2, "name": "Bob"},
			},
			wantOk: true,
		},
		{
			name:  "mixed array with objects",
			input: `[1, {key: "value"}, "string"]`,
			expected: []interface{}{
				1,
				map[string]interface{}{"key": "value"},
				"string",
			},
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseWSLValue(tt.input)
			if ok != tt.wantOk {
				t.Errorf("parseWSLValue() ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseWSLValue() = %#v, want %#v", got, tt.expected)
			}
		})
	}
}

func TestParseWSLValue_ObjectsWithArrays(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
		wantOk   bool
	}{
		{
			name:  "object with array value",
			input: `{users: [1, 2, 3]}`,
			expected: map[string]interface{}{
				"users": []interface{}{1, 2, 3},
			},
			wantOk: true,
		},
		{
			name:  "object with multiple array values",
			input: `{users: [1, 2, 3], tags: ["a", "b"]}`,
			expected: map[string]interface{}{
				"users": []interface{}{1, 2, 3},
				"tags":  []interface{}{"a", "b"},
			},
			wantOk: true,
		},
		{
			name:  "nested object with arrays",
			input: `{data: {items: [1, 2, 3]}}`,
			expected: map[string]interface{}{
				"data": map[string]interface{}{
					"items": []interface{}{1, 2, 3},
				},
			},
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseWSLValue(tt.input)
			if ok != tt.wantOk {
				t.Errorf("parseWSLValue() ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseWSLValue() = %#v, want %#v", got, tt.expected)
			}
		})
	}
}

func TestParseWSLValue_ComplexNested(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
		wantOk   bool
	}{
		{
			name: "complex nested structure",
			input: `{
				users: [
					{id: 1, name: "Alice", roles: ["admin", "user"]},
					{id: 2, name: "Bob", roles: ["user"]}
				],
				metadata: {
					tags: ["important", "urgent"],
					nested: {
						values: [true, false, true]
					}
				}
			}`,
			expected: map[string]interface{}{
				"users": []interface{}{
					map[string]interface{}{
						"id":    1,
						"name":  "Alice",
						"roles": []interface{}{"admin", "user"},
					},
					map[string]interface{}{
						"id":    2,
						"name":  "Bob",
						"roles": []interface{}{"user"},
					},
				},
				"metadata": map[string]interface{}{
					"tags": []interface{}{"important", "urgent"},
					"nested": map[string]interface{}{
						"values": []interface{}{true, false, true},
					},
				},
			},
			wantOk: true,
		},
		{
			name:  "array of arrays of objects",
			input: `[[{a: 1}], [{b: 2}]]`,
			expected: []interface{}{
				[]interface{}{map[string]interface{}{"a": 1}},
				[]interface{}{map[string]interface{}{"b": 2}},
			},
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseWSLValue(tt.input)
			if ok != tt.wantOk {
				t.Errorf("parseWSLValue() ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseWSLValue() = %#v, want %#v", got, tt.expected)
			}
		})
	}
}

func TestParseWSLValue_ArraysWithPlaceholders(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
		wantOk   bool
	}{
		{
			name:     "array with placeholders",
			input:    "[$User.ID, $Order.Total]",
			expected: []interface{}{"<<User.ID>>", "<<Order.Total>>"},
			wantOk:   true,
		},
		{
			name:     "mixed array with placeholders",
			input:    "[1, $User.ID, true, $Order.Total]",
			expected: []interface{}{1, "<<User.ID>>", true, "<<Order.Total>>"},
			wantOk:   true,
		},
		{
			name:  "object with array of placeholders",
			input: `{ids: [$User.ID, $Admin.ID]}`,
			expected: map[string]interface{}{
				"ids": []interface{}{"<<User.ID>>", "<<Admin.ID>>"},
			},
			wantOk: true,
		},
		{
			name:  "nested array in object in array with constants",
			input: `[{response: [{event: $constants.event, code: "Code202"}], statusCode: 202}]`,
			expected: []interface{}{
				map[string]interface{}{
					"response": []interface{}{
						map[string]interface{}{
							"event": "<<constants.event>>",
							"code":  "Code202",
						},
					},
					"statusCode": 202,
				},
			},
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseWSLValue(tt.input)
			if ok != tt.wantOk {
				t.Errorf("parseWSLValue() ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseWSLValue() = %#v, want %#v", got, tt.expected)
			}
		})
	}
}

func TestParseWSLValue_ExistingFunctionality(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
		wantOk   bool
	}{
		{
			name:     "number int",
			input:    "42",
			expected: 42,
			wantOk:   true,
		},
		{
			name:     "number float",
			input:    "3.14",
			expected: 3.14,
			wantOk:   true,
		},
		{
			name:     "boolean true",
			input:    "true",
			expected: true,
			wantOk:   true,
		},
		{
			name:     "boolean false",
			input:    "false",
			expected: false,
			wantOk:   true,
		},
		{
			name:     "quoted string",
			input:    `"hello"`,
			expected: "hello",
			wantOk:   true,
		},
		{
			name:     "placeholder",
			input:    "$User.ID",
			expected: "<<User.ID>>",
			wantOk:   true,
		},
		{
			name:     "special placeholder last response",
			input:    "$@.response",
			expected: "<<@.response>>",
			wantOk:   true,
		},
		{
			name:     "special placeholder last status",
			input:    "$?",
			expected: "<<?>>",
			wantOk:   true,
		},
		{
			name:     "special placeholder last error",
			input:    "$^",
			expected: "<<^>>",
			wantOk:   true,
		},
		{
			name:     "placeholder with special chars",
			input:    "$value!@#$%^&*_-+=~",
			expected: "<<value!@#$%^&*_-+=~>>",
			wantOk:   true,
		},
		{
			name:     "simple object",
			input:    "{key: value}",
			expected: map[string]interface{}{"key": "value"},
			wantOk:   true,
		},
		{
			name:     "nested object",
			input:    "{a: {b: {c: 1}}}",
			expected: map[string]interface{}{"a": map[string]interface{}{"b": map[string]interface{}{"c": 1}}},
			wantOk:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseWSLValue(tt.input)
			if ok != tt.wantOk {
				t.Errorf("parseWSLValue() ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseWSLValue() = %#v, want %#v", got, tt.expected)
			}
		})
	}
}

func TestParseWSLValue_DeeplyNestedEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
		wantOk   bool
	}{
		{
			name:  "triple nested array with mixed types",
			input: `[[[1, "a", true]]]`,
			expected: []interface{}{
				[]interface{}{
					[]interface{}{1, "a", true},
				},
			},
			wantOk: true,
		},
		{
			name:  "object with array containing object with array",
			input: `{outer: [{inner: [1, 2, 3]}]}`,
			expected: map[string]interface{}{
				"outer": []interface{}{
					map[string]interface{}{
						"inner": []interface{}{1, 2, 3},
					},
				},
			},
			wantOk: true,
		},
		{
			name:  "array with multiple objects containing arrays",
			input: `[{a: [1, 2]}, {b: [3, 4]}, {c: [5, 6]}]`,
			expected: []interface{}{
				map[string]interface{}{"a": []interface{}{1, 2}},
				map[string]interface{}{"b": []interface{}{3, 4}},
				map[string]interface{}{"c": []interface{}{5, 6}},
			},
			wantOk: true,
		},
		{
			name:  "nested placeholders in deep structure",
			input: `{level1: {level2: {level3: $deep.value}}}`,
			expected: map[string]interface{}{
				"level1": map[string]interface{}{
					"level2": map[string]interface{}{
						"level3": "<<deep.value>>",
					},
				},
			},
			wantOk: true,
		},
		{
			name:  "array with empty objects and arrays",
			input: `[{}, [], {a: []}]`,
			expected: []interface{}{
				map[string]interface{}{},
				[]interface{}{},
				map[string]interface{}{"a": []interface{}{}},
			},
			wantOk: true,
		},
		{
			name:  "complex mix matching the issue pattern",
			input: `[{items: [{id: 1, name: $item.name}], count: 10}]`,
			expected: []interface{}{
				map[string]interface{}{
					"items": []interface{}{
						map[string]interface{}{
							"id":   1,
							"name": "<<item.name>>",
						},
					},
					"count": 10,
				},
			},
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseWSLValue(tt.input)
			if ok != tt.wantOk {
				t.Errorf("parseWSLValue() ok = %v, want %v", ok, tt.wantOk)
				return
			}
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("parseWSLValue() = %#v, want %#v", got, tt.expected)
			}
		})
	}
}
