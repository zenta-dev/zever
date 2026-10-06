package openapi

import (
	"testing"
)

func TestJSONSchemaForEntity(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, `enum Status { active, inactive }

	entity Task {
		id: uuid @primary
		status: Status
		inline: enum(x, y)
	}

	service Svc {
		rpc GetTask(id: uuid) -> Task {
			http: GET "/v1/tasks/{id}"
			auth: required
		}
	}`)

	out, err := JSONSchemaForEntity(schema, "default", "Task")
	if err != nil {
		t.Fatalf("JSONSchemaForEntity: %v", err)
	}

	if out["$ref"] != "#/definitions/Task" {
		t.Fatalf("$ref = %v, want #/definitions/Task", out["$ref"])
	}

	defs, ok := out["definitions"].(map[string]any)
	if !ok {
		t.Fatalf("definitions type = %T, want map", out["definitions"])
	}

	task, ok := defs["Task"].(map[string]any)
	if !ok {
		t.Fatalf("Task definition type = %T, want map", defs["Task"])
	}

	props, ok := task["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties type = %T, want map", task["properties"])
	}

	status, ok := props["status"].(map[string]any)
	if !ok {
		t.Fatalf("status type = %T, want map", props["status"])
	}

	if status["$ref"] != "#/definitions/Status" {
		t.Fatalf("status schema = %v, want $ref #/definitions/Status", status)
	}

	if _, ok := defs["Status"].(map[string]any); !ok {
		t.Fatalf("definitions missing Status: %v", keysOfAny(defs))
	}
}

func TestJSONSchemaForEntity_notFound(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, `entity Task {
		id: uuid @primary
	}`)

	if _, err := JSONSchemaForEntity(schema, "default", "Missing"); err == nil {
		t.Fatal("expected error for unknown entity, got nil")
	}

	if _, err := JSONSchemaForEntity(schema, "nope", "Task"); err == nil {
		t.Fatal("expected error for unknown module, got nil")
	}
}

func TestJSONSchemaForEntity_nilSchema(t *testing.T) {
	t.Parallel()

	if _, err := JSONSchemaForEntity(nil, "default", "Task"); err == nil {
		t.Fatal("expected error for nil schema, got nil")
	}
}
