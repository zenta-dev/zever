package flag

import (
	"context"
	"encoding/json"
	"testing"
)

type stubFlag struct{}

func (s *stubFlag) Bool(_ context.Context, _ string, fallback bool) (bool, error) {
	return fallback, nil
}

func (s *stubFlag) String(_ context.Context, _ string, fallback string) (string, error) {
	return fallback, nil
}

func (s *stubFlag) Int(_ context.Context, _ string, fallback int) (int, error) {
	return fallback, nil
}

func (s *stubFlag) JSON(_ context.Context, _ string, _ any, _ any) error { return nil }

func (s *stubFlag) Close() error { return nil }

func TestWithEvalContext_roundtrip_present(t *testing.T) {
	t.Parallel()
	ec := EvalContext{RandomizationID: "user-123", Signals: map[string]any{"plan": "pro"}}
	ctx := WithEvalContext(t.Context(), ec)
	got, ok := EvalContextFrom(ctx)
	if !ok {
		t.Fatal("EvalContextFrom = false, want true")
	}
	if got.RandomizationID != ec.RandomizationID {
		t.Errorf("RandomizationID = %q want %q", got.RandomizationID, ec.RandomizationID)
	}
	if got.Signals["plan"] != "pro" {
		t.Errorf("Signals = %v want plan=pro", got.Signals)
	}
}

func TestEvalContextFrom_missing_returnsFalse(t *testing.T) {
	t.Parallel()
	if _, ok := EvalContextFrom(t.Context()); ok {
		t.Error("EvalContextFrom = true, want false")
	}
}

// jsonStubFlag mirrors the static adapter's JSON contract closely enough to
// exercise GetJSON: string values are raw JSON documents, missing keys fill
// out from a non-nil fallback, and decode failures surface as errors.
type jsonStubFlag struct {
	values map[string]any
}

func (s *jsonStubFlag) Bool(context.Context, string, bool) (bool, error) { return false, nil }

func (s *jsonStubFlag) String(context.Context, string, string) (string, error) { return "", nil }

func (s *jsonStubFlag) Int(context.Context, string, int) (int, error) { return 0, nil }

func (s *jsonStubFlag) JSON(_ context.Context, key string, out any, fallback any) error {
	v, ok := s.values[key]
	if !ok {
		if fallback == nil {
			return nil
		}

		b, err := json.Marshal(fallback)
		if err != nil {
			return err
		}

		return json.Unmarshal(b, out)
	}

	var b []byte
	if str, ok := v.(string); ok {
		b = []byte(str)
	} else {
		var err error
		if b, err = json.Marshal(v); err != nil {
			return err
		}
	}

	return json.Unmarshal(b, out)
}

func (s *jsonStubFlag) Close() error { return nil }

type jsonConfig struct {
	Port int `json:"port"`
}

func TestGetJSON_present_absent_and_wrongType(t *testing.T) {
	t.Parallel()

	f := &jsonStubFlag{values: map[string]any{
		"app":    `{"port":8080}`,
		"scalar": `"nope"`,
	}}

	got, err := GetJSON(t.Context(), f, "app", jsonConfig{})
	if err != nil {
		t.Fatalf("GetJSON(present) err = %v, want nil", err)
	}
	if got.Port != 8080 {
		t.Errorf("GetJSON(present) = %+v, want Port 8080", got)
	}

	fallback := jsonConfig{Port: 9}

	got, err = GetJSON(t.Context(), f, "missing", fallback)
	if err != nil {
		t.Fatalf("GetJSON(absent) err = %v, want nil", err)
	}
	if got != fallback {
		t.Errorf("GetJSON(absent) = %+v, want fallback %+v", got, fallback)
	}

	got, err = GetJSON(t.Context(), f, "scalar", fallback)
	if err == nil {
		t.Fatal("GetJSON(wrong type) err = nil, want decode error")
	}
	if got != fallback {
		t.Errorf("GetJSON(wrong type) = %+v, want fallback %+v", got, fallback)
	}
}
