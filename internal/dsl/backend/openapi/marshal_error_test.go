package openapi

import (
	"errors"
	"strings"
	"testing"
)

// Tests in this file swap the jsonMarshalIndent test seam (see openapi.go),
// so they must NOT use t.Parallel: every other test in this package runs in
// parallel, and parallel tests never overlap with sequential ones, which
// keeps the seam swap race-free without a mutex.

var errTestMarshal = errors.New("test marshal failure")

// swapMarshal swaps the jsonMarshalIndent seam, restoring it via t.Cleanup.
func swapMarshal(t *testing.T, fn func(v any, prefix, indent string) ([]byte, error)) {
	t.Helper()

	orig := jsonMarshalIndent
	jsonMarshalIndent = fn
	t.Cleanup(func() { jsonMarshalIndent = orig })
}

// TestGenerate_moduleMarshalError_propagates covers Generate's per-module
// json.MarshalIndent error branch (openapi.go): the seam fails on the first
// call, so the module-spec marshal reports an error naming the module.
func TestGenerate_moduleMarshalError_propagates(t *testing.T) {
	schema := compileSchema(t, `entity Widget {
		id: uuid @primary
	}

	service Svc {
		rpc GetWidget(id: uuid) -> Widget {
			http: GET "/v1/widgets/{id}"
			auth: required
		}
	}`)

	swapMarshal(t, func(_ any, _, _ string) ([]byte, error) {
		return nil, errTestMarshal
	})

	_, err := New().Generate(schema)
	if err == nil {
		t.Fatal("Generate: expected a module marshal error, got nil")
	}

	if !errors.Is(err, errTestMarshal) {
		t.Fatalf("Generate error = %v, want wrapped %v", err, errTestMarshal)
	}

	if msg := err.Error(); !strings.Contains(msg, "marshal default spec") {
		t.Fatalf("error %q should name the failing module spec", msg)
	}
}

// TestGenerate_mergedMarshalError_propagates covers Generate's merged-spec
// json.MarshalIndent error branch (openapi.go): the seam succeeds for the
// per-module marshal but fails for the merged marshal. This branch is
// unreachable via real inputs (any value failing the merged marshal would
// already fail its own module marshal first), hence the seam.
func TestGenerate_mergedMarshalError_propagates(t *testing.T) {
	schema := compileSchema(t, `entity Widget {
		id: uuid @primary
	}

	service Svc {
		rpc GetWidget(id: uuid) -> Widget {
			http: GET "/v1/widgets/{id}"
			auth: required
		}
	}`)

	orig := jsonMarshalIndent
	calls := 0
	jsonMarshalIndent = func(v any, prefix, indent string) ([]byte, error) {
		calls++
		if calls == 1 {
			return orig(v, prefix, indent)
		}
		return nil, errTestMarshal
	}
	t.Cleanup(func() { jsonMarshalIndent = orig })

	_, err := New().Generate(schema)
	if err == nil {
		t.Fatal("Generate: expected a merged marshal error, got nil")
	}

	if !errors.Is(err, errTestMarshal) {
		t.Fatalf("Generate error = %v, want wrapped %v", err, errTestMarshal)
	}

	if msg := err.Error(); !strings.Contains(msg, "marshal merged spec") {
		t.Fatalf("error %q should name the merged spec", msg)
	}
}
