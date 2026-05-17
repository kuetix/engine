package workflow

import (
	"testing"

	"github.com/kuetix/engine/internal/wsl"
)

// TestWSLArrays_EndToEnd demonstrates comprehensive array support in WSL
// with realistic workflow scenarios
func TestWSLArrays_EndToEnd(t *testing.T) {
	src := `
module data_processing

workflow batch_processor {
    start: load_data
    
    state load_data {
        action loader.loadBatch(
            ids: [1, 2, 3, 4, 5],
            sources: ["db", "api", "cache"],
            config: {
                timeout: 30,
                retries: 3,
                filters: ["active", "verified"]
            }
        )
        on success -> validate_data
        on error -> handle_error
    }
    
    state validate_data {
        action validator.check({
            required_fields: ["id", "name", "email"],
            optional_fields: ["phone", "address"],
            rules: [
                {field: "id", type: "number", min: 1},
                {field: "email", type: "string", pattern: "email"}
            ]
        })
        on success -> process_batch
        on error -> handle_error
    }
    
    state process_batch {
        action processor.transform(
            data: $Result.records,
            operations: [
                {type: "filter", condition: $constants.filter_rule},
                {type: "map", fields: ["id", "name", "email"]},
                {type: "sort", by: "id", order: "asc"}
            ],
            options: {
                parallel: true,
                batch_size: 100,
                error_handling: "continue"
            }
        )
        on success -> save_results
        on error -> handle_error
    }
    
    state save_results {
        action storage.save({
            destination: "processed",
            format: "json",
            metadata: {
                processed_at: $timestamp,
                processor_version: "2.0",
                tags: ["batch", "processed", $constants.environment]
            }
        })
        on success -> complete
        on error -> handle_error
    }
    
    state handle_error {
        end fail code="PROCESSING_ERROR"
    }
    
    state complete {
        end ok
    }
}
`

	// Parse the workflow
	mod, graphs, err := wsl.ParseAll(src, "")
	if err != nil {
		t.Fatalf("ParseAll error: %v", err)
	}

	if mod.Name != "data_processing" {
		t.Errorf("module name: got %s, want data_processing", mod.Name)
	}

	if len(mod.Workflows) != 1 {
		t.Fatalf("expected 1 workflow, got %d", len(mod.Workflows))
	}

	wf := mod.Workflows[0]
	if wf.Name != "batch_processor" {
		t.Errorf("workflow name: got %s, want batch_processor", wf.Name)
	}

	// Check graph was built
	graph, ok := graphs["batch_processor"]
	if !ok {
		t.Fatal("graph for batch_processor not found")
	}

	if graph.WorkflowName != "batch_processor" {
		t.Errorf("graph workflow name: got %s, want batch_processor", graph.WorkflowName)
	}

	// Verify state load_data
	loadDataState, ok := wf.States["load_data"]
	if !ok {
		t.Fatal("load_data state not found")
	}

	if loadDataState.Action == nil {
		t.Fatal("load_data has no action")
	}

	if len(loadDataState.Action.Args) < 1 {
		t.Fatalf("expected at least 1 arg, got %d", len(loadDataState.Action.Args))
	}

	// Use argsToOptions to properly parse named parameters
	opts := argsToOptions(loadDataState.Action.Args)

	// Verify ids array
	ids, ok := opts["ids"].([]interface{})
	if !ok {
		t.Fatalf("expected ids to be array, got %T", opts["ids"])
	}
	if len(ids) != 5 {
		t.Errorf("expected 5 ids, got %d", len(ids))
	}

	// Verify sources array
	sources, ok := opts["sources"].([]interface{})
	if !ok {
		t.Fatalf("expected sources to be array, got %T", opts["sources"])
	}
	if len(sources) != 3 {
		t.Errorf("expected 3 sources, got %d", len(sources))
	}

	// Verify config object with nested array
	config, ok := opts["config"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected config to be object, got %T", opts["config"])
	}

	filters, ok := config["filters"].([]interface{})
	if !ok {
		t.Fatalf("expected filters to be array, got %T", config["filters"])
	}
	if len(filters) != 2 {
		t.Errorf("expected 2 filters, got %d", len(filters))
	}

	// Verify validate_data state with array of objects
	validateState, ok := wf.States["validate_data"]
	if !ok {
		t.Fatal("validate_data state not found")
	}

	opts = argsToOptions(validateState.Action.Args)

	// Verify rules array with objects
	rules, ok := opts["rules"].([]interface{})
	if !ok {
		t.Fatalf("expected rules to be array, got %T", opts["rules"])
	}

	if len(rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(rules))
	}

	// Verify first rule is an object
	rule1, ok := rules[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected rule to be object, got %T", rules[0])
	}

	if rule1["field"] != "id" {
		t.Errorf("rule1 field: got %v, want id", rule1["field"])
	}

	// Verify process_batch state with placeholder and arrays
	processBatchState, ok := wf.States["process_batch"]
	if !ok {
		t.Fatal("process_batch state not found")
	}

	opts = argsToOptions(processBatchState.Action.Args)

	// Verify operations array with objects
	operations, ok := opts["operations"].([]interface{})
	if !ok {
		t.Fatalf("expected operations to be array, got %T", opts["operations"])
	}

	if len(operations) != 3 {
		t.Errorf("expected 3 operations, got %d", len(operations))
	}

	// Verify save_results state with mixed types
	saveResultsState, ok := wf.States["save_results"]
	if !ok {
		t.Fatal("save_results state not found")
	}

	opts = argsToOptions(saveResultsState.Action.Args)

	// Verify metadata with tags array including placeholder
	metadata, ok := opts["metadata"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected metadata to be object, got %T", opts["metadata"])
	}

	tags, ok := metadata["tags"].([]interface{})
	if !ok {
		t.Fatalf("expected tags to be array, got %T", metadata["tags"])
	}

	if len(tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(tags))
	}

	// Verify placeholder in array was converted
	if tags[2] != "<<constants.environment>>" {
		t.Errorf("tag[2]: got %v, want <<constants.environment>>", tags[2])
	}

	// Verify all states are present in the workflow
	expectedStates := []string{"load_data", "validate_data", "process_batch", "save_results", "handle_error", "complete"}
	if len(wf.States) != len(expectedStates) {
		t.Errorf("expected %d states, got %d", len(expectedStates), len(wf.States))
	}

	for _, stateName := range expectedStates {
		if _, ok := wf.States[stateName]; !ok {
			t.Errorf("state %s not found in workflow", stateName)
		}
	}
}
