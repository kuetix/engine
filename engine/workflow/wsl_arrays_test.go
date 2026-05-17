package workflow

import (
	"testing"

	"github.com/kuetix/engine/internal/wsl"
)

func TestWSLIntegration_ArraysInActions(t *testing.T) {
	src := `
module test_arrays

workflow process_data {
    start: initialize
    
    state initialize {
        action setup([1, 2, 3, 4, 5])
        on success -> process
    }
    
    state process {
        action transform({
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
        })
        on success -> finish
    }
    
    state finish {
        end ok
    }
}
`

	// Parse the WSL
	mod, _, err := wsl.ParseAll(src, "")
	if err != nil {
		t.Fatalf("ParseAll error: %v", err)
	}

	if len(mod.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(mod.Workflows))
	}

	wf := mod.Workflows[0]
	if wf.Name != "process_data" {
		t.Fatalf("workflow name: got %q, want process_data", wf.Name)
	}

	// Check initialize state has array argument
	initState, ok := wf.States["initialize"]
	if !ok {
		t.Fatal("initialize state not found")
	}

	if initState.Action == nil {
		t.Fatal("initialize state has no action")
	}

	if len(initState.Action.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(initState.Action.Args))
	}

	// The arg should be parseable as array
	argRaw := initState.Action.Args[0].Raw
	val, ok := parseWSLValue(argRaw)
	if !ok {
		t.Fatalf("failed to parse arg: %s", argRaw)
	}

	arr, ok := val.([]interface{})
	if !ok {
		t.Fatalf("expected array, got %T", val)
	}

	if len(arr) != 5 {
		t.Fatalf("expected array length 5, got %d", len(arr))
	}

	// Check process state has complex object with arrays
	processState, ok := wf.States["process"]
	if !ok {
		t.Fatal("process state not found")
	}

	if processState.Action == nil {
		t.Fatal("process state has no action")
	}

	if len(processState.Action.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(processState.Action.Args))
	}

	// The arg should be parseable as object with arrays
	argRaw = processState.Action.Args[0].Raw
	val, ok = parseWSLValue(argRaw)
	if !ok {
		t.Fatalf("failed to parse arg: %s", argRaw)
	}

	obj, ok := val.(map[string]interface{})
	if !ok {
		t.Fatalf("expected object, got %T", val)
	}

	// Check users array
	users, ok := obj["users"].([]interface{})
	if !ok {
		t.Fatalf("expected users to be array, got %T", obj["users"])
	}

	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}

	// Check first user
	user1, ok := users[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected user to be object, got %T", users[0])
	}

	if user1["id"] != 1 {
		t.Errorf("user1 id: got %v, want 1", user1["id"])
	}

	if user1["name"] != "Alice" {
		t.Errorf("user1 name: got %v, want Alice", user1["name"])
	}

	// Check user1 roles array
	roles1, ok := user1["roles"].([]interface{})
	if !ok {
		t.Fatalf("expected roles to be array, got %T", user1["roles"])
	}

	if len(roles1) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles1))
	}

	if roles1[0] != "admin" {
		t.Errorf("role 0: got %v, want admin", roles1[0])
	}

	// Check metadata
	metadata, ok := obj["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected metadata to be object, got %T", obj["metadata"])
	}

	// Check tags array
	tags, ok := metadata["tags"].([]interface{})
	if !ok {
		t.Fatalf("expected tags to be array, got %T", metadata["tags"])
	}

	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}

	// Check nested values array
	nested, ok := metadata["nested"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested to be object, got %T", metadata["nested"])
	}

	values, ok := nested["values"].([]interface{})
	if !ok {
		t.Fatalf("expected values to be array, got %T", nested["values"])
	}

	if len(values) != 3 {
		t.Fatalf("expected 3 values, got %d", len(values))
	}

	if values[0] != true || values[1] != false || values[2] != true {
		t.Errorf("values: got %v, want [true, false, true]", values)
	}
}

func TestWSLIntegration_ArraysWithPlaceholders(t *testing.T) {
	src := `
module test_placeholders

workflow check_ids {
    start: validate
    
    state validate {
        action check([$User.ID, $Order.ID, $Product.ID])
        on success -> done
    }
    
    state done {
        end ok
    }
}
`

	mod, _, err := wsl.ParseAll(src, "")
	if err != nil {
		t.Fatalf("ParseAll error: %v", err)
	}

	validateState, ok := mod.Workflows[0].States["validate"]
	if !ok {
		t.Fatal("validate state not found")
	}

	argRaw := validateState.Action.Args[0].Raw
	val, ok := parseWSLValue(argRaw)
	if !ok {
		t.Fatalf("failed to parse arg: %s", argRaw)
	}

	arr, ok := val.([]interface{})
	if !ok {
		t.Fatalf("expected array, got %T", val)
	}

	if len(arr) != 3 {
		t.Fatalf("expected array length 3, got %d", len(arr))
	}

	// Check placeholders are converted correctly
	expected := []string{"<<User.ID>>", "<<Order.ID>>", "<<Product.ID>>"}
	for i, exp := range expected {
		if arr[i] != exp {
			t.Errorf("arr[%d]: got %v, want %s", i, arr[i], exp)
		}
	}
}

func TestWSLIntegration_EmptyArray(t *testing.T) {
	src := `
module test_empty

workflow test {
    start: init
    
    state init {
        action setup([])
        on success -> done
    }
    
    state done {
        end ok
    }
}
`

	mod, _, err := wsl.ParseAll(src, "")
	if err != nil {
		t.Fatalf("ParseAll error: %v", err)
	}

	initState, ok := mod.Workflows[0].States["init"]
	if !ok {
		t.Fatal("init state not found")
	}

	argRaw := initState.Action.Args[0].Raw
	val, ok := parseWSLValue(argRaw)
	if !ok {
		t.Fatalf("failed to parse arg: %s", argRaw)
	}

	arr, ok := val.([]interface{})
	if !ok {
		t.Fatalf("expected array, got %T", val)
	}

	if len(arr) != 0 {
		t.Fatalf("expected empty array, got length %d", len(arr))
	}
}

func TestWSLIntegration_MultipleArrayArgs(t *testing.T) {
	src := `
module test_multi

workflow test {
    start: process
    
    state process {
        action transform([1, 2, 3], ["a", "b", "c"], [true, false])
        on success -> done
    }
    
    state done {
        end ok
    }
}
`

	mod, _, err := wsl.ParseAll(src, "")
	if err != nil {
		t.Fatalf("ParseAll error: %v", err)
	}

	processState, ok := mod.Workflows[0].States["process"]
	if !ok {
		t.Fatal("process state not found")
	}

	if len(processState.Action.Args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(processState.Action.Args))
	}

	// First arg: [1, 2, 3]
	val1, ok := parseWSLValue(processState.Action.Args[0].Raw)
	if !ok {
		t.Fatal("failed to parse first arg")
	}
	arr1 := val1.([]interface{})
	if len(arr1) != 3 || arr1[0] != 1 {
		t.Errorf("first arg: got %v", arr1)
	}

	// Second arg: ["a", "b", "c"]
	val2, ok := parseWSLValue(processState.Action.Args[1].Raw)
	if !ok {
		t.Fatal("failed to parse second arg")
	}
	arr2 := val2.([]interface{})
	if len(arr2) != 3 || arr2[0] != "a" {
		t.Errorf("second arg: got %v", arr2)
	}

	// Third arg: [true, false]
	val3, ok := parseWSLValue(processState.Action.Args[2].Raw)
	if !ok {
		t.Fatal("failed to parse third arg")
	}
	arr3 := val3.([]interface{})
	if len(arr3) != 2 || arr3[0] != true {
		t.Errorf("third arg: got %v", arr3)
	}
}

func TestWSLIntegration_NestedArraysWithConstants(t *testing.T) {
	src := `
module test_nested

const {
  event: "UserCreated"
}

workflow test {
    start: MassiveHello
    
    state MassiveHello {
        action converse/speak.Say(on: message, v: [{response: [{event: $constants.event, code: "Code202"}], statusCode: 202}]) as ResponseAsValue
        on success -> Response
    }
    
    state Response {
        action response/Value(v: ResponseAsValue)
        end ok
    }
}
`
	// Parse the WSL
	mod, graphs, err := wsl.ParseAll(src, "")
	if err != nil {
		t.Fatalf("ParseAll error: %v", err)
	}

	if len(mod.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(mod.Workflows))
	}

	wf := mod.Workflows[0]
	if wf.Name != "test" {
		t.Fatalf("workflow name: got %q, want test", wf.Name)
	}

	// Check that constants were parsed
	graph, ok := graphs["test"]
	if !ok {
		t.Fatal("graph for test workflow not found")
	}

	if graph.Constants == nil {
		t.Fatal("expected constants to be parsed")
	}

	if graph.Constants["event"] != "UserCreated" {
		t.Errorf("constants.event: got %v, want UserCreated", graph.Constants["event"])
	}

	// Check MassiveHello state
	massiveHelloState, ok := wf.States["MassiveHello"]
	if !ok {
		t.Fatal("MassiveHello state not found")
	}

	if massiveHelloState.Action == nil {
		t.Fatal("MassiveHello state has no action")
	}

	// Parse the v argument which contains the nested structure
	opts := argsToOptions(massiveHelloState.Action.Args)

	// Verify the nested structure
	vValue, ok := opts["v"]
	if !ok {
		t.Fatal("v parameter not found in action args")
	}

	// Should be an array
	vArray, ok := vValue.([]interface{})
	if !ok {
		t.Fatalf("expected v to be array, got %T", vValue)
	}

	if len(vArray) != 1 {
		t.Fatalf("expected v array length 1, got %d", len(vArray))
	}

	// First element should be an object
	firstObj, ok := vArray[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected first element to be object, got %T", vArray[0])
	}

	// Check statusCode
	if firstObj["statusCode"] != 202 {
		t.Errorf("statusCode: got %v, want 202", firstObj["statusCode"])
	}

	// Check response array
	response, ok := firstObj["response"].([]interface{})
	if !ok {
		t.Fatalf("expected response to be array, got %T", firstObj["response"])
	}

	if len(response) != 1 {
		t.Fatalf("expected response array length 1, got %d", len(response))
	}

	// Check nested object in response array
	nestedObj, ok := response[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested element to be object, got %T", response[0])
	}

	// Verify that event is a placeholder (will be resolved later)
	if nestedObj["event"] != "<<constants.event>>" {
		t.Errorf("event: got %v, want <<constants.event>>", nestedObj["event"])
	}

	// Verify that code is the string literal
	if nestedObj["code"] != "Code202" {
		t.Errorf("code: got %v, want Code202", nestedObj["code"])
	}

	t.Logf("Successfully parsed nested structure:")
	t.Logf("  v: %#v", vValue)
	t.Logf("  First object statusCode: %v", firstObj["statusCode"])
	t.Logf("  Response array: %#v", response)
	t.Logf("  Nested object event (placeholder): %v", nestedObj["event"])
	t.Logf("  Nested object code: %v", nestedObj["code"])
}
