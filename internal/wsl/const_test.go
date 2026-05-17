package wsl

import (
	"testing"
)

func TestParseConstBlock_Nested(t *testing.T) {
	src := `module example

const {
    event: "greet",
    description: "A simple hello world workflow example",
    version: "1.0.0",
    maxRetries: 3,
    enabled: true,
    cfg: {
        apiEndpoint: "https://api.example.com",
        timeout: 30000,
        retries: 3,
        headers: [
            { key: "Content-Type", value: "application/json" },
            { key: "Accept", value: "application/json" }
        ]
    }
}

workflow test {
  start: Initial
  state Initial {
    action services/common/response.ResponseValue(value: "test", statusCode: 200) as Result
    end ok
  }
}`

	cst, err := ParseCST(src, "")
	if err != nil {
		t.Fatalf("ParseCST failed: %v", err)
	}

	if cst.Constants == nil {
		t.Fatal("expected const block, got nil")
	}

	if len(cst.Constants.Entries) != 6 {
		t.Fatalf("expected 6 const entries, got %d", len(cst.Constants.Entries))
	}

	// Check simple values
	checkScalarEntry(t, cst.Constants.Entries[0], "event", "greet")
	checkScalarEntry(t, cst.Constants.Entries[1], "description", "A simple hello world workflow example")
	checkScalarEntry(t, cst.Constants.Entries[2], "version", "1.0.0")
	checkScalarEntry(t, cst.Constants.Entries[3], "maxRetries", "3")
	checkScalarEntry(t, cst.Constants.Entries[4], "enabled", "true")

	// Check nested object
	cfgEntry := cst.Constants.Entries[5]
	if cfgEntry.Key.Lexeme != "cfg" {
		t.Errorf("expected key 'cfg', got '%s'", cfgEntry.Key.Lexeme)
	}
	if cfgEntry.Val.Kind != CSTValueObject {
		t.Errorf("expected object value, got kind %d", cfgEntry.Val.Kind)
	}
	if cfgEntry.Val.Object == nil {
		t.Fatal("expected object, got nil")
	}

	obj := cfgEntry.Val.Object
	if len(obj.Entries) != 4 {
		t.Fatalf("expected 4 object entries, got %d", len(obj.Entries))
	}

	// Check object properties
	checkObjectScalarEntry(t, obj.Entries[0], "apiEndpoint", "https://api.example.com")
	checkObjectScalarEntry(t, obj.Entries[1], "timeout", "30000")
	checkObjectScalarEntry(t, obj.Entries[2], "retries", "3")

	// Check nested array
	headersEntry := obj.Entries[3]
	if headersEntry.Key.Lexeme != "headers" {
		t.Errorf("expected key 'headers', got '%s'", headersEntry.Key.Lexeme)
	}
	if headersEntry.Val.Kind != CSTValueArray {
		t.Errorf("expected array value, got kind %d", headersEntry.Val.Kind)
	}
	if headersEntry.Val.Array == nil {
		t.Fatal("expected array, got nil")
	}

	arr := headersEntry.Val.Array
	if len(arr.Values) != 2 {
		t.Fatalf("expected 2 array elements, got %d", len(arr.Values))
	}

	// Check array elements (should be objects)
	for i, val := range arr.Values {
		if val.Kind != CSTValueObject {
			t.Errorf("array element %d: expected object, got kind %d", i, val.Kind)
		}
	}
}

func TestBuildAST_NestedConst(t *testing.T) {
	src := `module example

const {
    event: "greet",
    description: "A simple hello world workflow example",
    version: "1.0.0",
    maxRetries: 3,
    enabled: true,
    cfg: {
        apiEndpoint: "https://api.example.com",
        timeout: 30000,
        retries: 3,
        headers: [
            { key: "Content-Type", value: "application/json" },
            { key: "Accept", value: "application/json" }
        ]
    }
}

workflow test {
  start: Initial
  state Initial {
    action services/common/response.ResponseValue(value: "test", statusCode: 200) as Result
    end ok
  }
}`

	cst, err := ParseCST(src, "")
	if err != nil {
		t.Fatalf("ParseCST failed: %v", err)
	}

	ast, err := BuildAST(cst)
	if err != nil {
		t.Fatalf("BuildAST failed: %v", err)
	}

	if len(ast.Constants) != 6 {
		t.Fatalf("expected 6 constants, got %d", len(ast.Constants))
	}

	// Check simple values
	checkASTConstant(t, ast.Constants[0], "event", "greet")
	checkASTConstant(t, ast.Constants[1], "description", "A simple hello world workflow example")
	checkASTConstant(t, ast.Constants[2], "version", "1.0.0")
	checkASTConstant(t, ast.Constants[3], "maxRetries", int64(3))
	checkASTConstant(t, ast.Constants[4], "enabled", true)

	// Check nested object
	cfgConst := ast.Constants[5]
	if cfgConst.Name != "cfg" {
		t.Errorf("expected name 'cfg', got '%s'", cfgConst.Name)
	}

	cfgMap, ok := cfgConst.Value.(map[string]interface{})
	if !ok {
		t.Fatalf("expected cfg to be map[string]interface{}, got %T", cfgConst.Value)
	}

	if cfgMap["apiEndpoint"] != "https://api.example.com" {
		t.Errorf("expected apiEndpoint='https://api.example.com', got '%v'", cfgMap["apiEndpoint"])
	}
	if cfgMap["timeout"] != int64(30000) {
		t.Errorf("expected timeout=30000, got '%v'", cfgMap["timeout"])
	}
	if cfgMap["retries"] != int64(3) {
		t.Errorf("expected retries=3, got '%v'", cfgMap["retries"])
	}

	// Check headers array
	headers, ok := cfgMap["headers"].([]interface{})
	if !ok {
		t.Fatalf("expected headers to be []interface{}, got %T", cfgMap["headers"])
	}
	if len(headers) != 2 {
		t.Fatalf("expected 2 headers, got %d", len(headers))
	}

	// Check first header object
	header1, ok := headers[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected header[0] to be map[string]interface{}, got %T", headers[0])
	}
	if header1["key"] != "Content-Type" {
		t.Errorf("expected key='Content-Type', got '%v'", header1["key"])
	}
	if header1["value"] != "application/json" {
		t.Errorf("expected value='application/json', got '%v'", header1["value"])
	}
}

// Helper functions
func checkScalarEntry(t *testing.T, entry CSTConstEntry, expectedKey, expectedValue string) {
	if entry.Key.Lexeme != expectedKey {
		t.Errorf("expected key '%s', got '%s'", expectedKey, entry.Key.Lexeme)
	}
	if entry.Val.Kind != CSTValueScalar {
		t.Errorf("expected scalar value for key '%s', got kind %d", expectedKey, entry.Val.Kind)
	}
	if entry.Val.Token == nil {
		t.Fatalf("expected token for key '%s', got nil", expectedKey)
	}
	if entry.Val.Token.Lexeme != expectedValue {
		t.Errorf("expected value '%s' for key '%s', got '%s'", expectedValue, expectedKey, entry.Val.Token.Lexeme)
	}
}

func checkObjectScalarEntry(t *testing.T, entry CSTObjectEntry, expectedKey, expectedValue string) {
	if entry.Key.Lexeme != expectedKey {
		t.Errorf("expected key '%s', got '%s'", expectedKey, entry.Key.Lexeme)
	}
	if entry.Val.Kind != CSTValueScalar {
		t.Errorf("expected scalar value for key '%s', got kind %d", expectedKey, entry.Val.Kind)
	}
	if entry.Val.Token == nil {
		t.Fatalf("expected token for key '%s', got nil", expectedKey)
	}
	if entry.Val.Token.Lexeme != expectedValue {
		t.Errorf("expected value '%s' for key '%s', got '%s'", expectedValue, expectedKey, entry.Val.Token.Lexeme)
	}
}

func checkASTConstant(t *testing.T, constant Constant, expectedName string, expectedValue interface{}) {
	if constant.Name != expectedName {
		t.Errorf("expected name '%s', got '%s'", expectedName, constant.Name)
	}
	if constant.Value != expectedValue {
		t.Errorf("expected value '%v' for constant '%s', got '%v'", expectedValue, expectedName, constant.Value)
	}
}
