package firebase

import (
	"errors"
	"sync"
	"testing"

	"firebase.google.com/go/v4/remoteconfig"
)

// mustClientFromTemplate builds a hermetic client from a raw server-template
// JSON document.
func mustClientFromTemplate(t *testing.T, tplJSON string) *client {
	t.Helper()

	tpl, err := (&remoteconfig.Client{}).InitServerTemplate(map[string]any{}, tplJSON)
	if err != nil {
		t.Fatalf("InitServerTemplate() error = %v", err)
	}

	return &client{eval: tpl.Evaluate}
}

// TestEdgeParseFirebaseIntTable checks integer coercion boundaries.
func TestEdgeParseFirebaseIntTable(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want int
		err  bool
	}{
		{"plain", "42", 42, false},
		{"padded", "  42  ", 42, false},
		{"plus", "+5", 5, false},
		{"negative", "-7", -7, false},
		{"zero", "0", 0, false},
		{"float truncates", "3.9", 3, false},
		{"negative float truncates", "-2.7", -2, false},
		{"exponent", "1e3", 1000, false},
		{"empty", "", 0, true},
		{"blank", "   ", 0, true},
		{"letters", "abc", 0, true},
		{"overflow", "9999999999999999999999", 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseFirebaseInt(tc.in)
			if tc.err {
				if err == nil {
					t.Fatalf("parseFirebaseInt(%q) = %d, nil; want error", tc.in, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseFirebaseInt(%q) error = %v", tc.in, err)
			}

			if got != tc.want {
				t.Fatalf("parseFirebaseInt(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// TestEdgeBoolTruthyTable checks the SDK truthy set boundary.
func TestEdgeBoolTruthyTable(t *testing.T) {
	t.Parallel()

	const tpl = `{"parameters":{` +
		`"b1":{"defaultValue":{"value":"1"}},` +
		`"b2":{"defaultValue":{"value":"true"}},` +
		`"b3":{"defaultValue":{"value":"t"}},` +
		`"b4":{"defaultValue":{"value":"yes"}},` +
		`"b5":{"defaultValue":{"value":"y"}},` +
		`"b6":{"defaultValue":{"value":"on"}},` +
		`"b7":{"defaultValue":{"value":"0"}},` +
		`"b8":{"defaultValue":{"value":"off"}},` +
		`"b9":{"defaultValue":{"value":"no"}},` +
		`"b10":{"defaultValue":{"value":"maybe"}}` +
		`}}`

	c := mustClientFromTemplate(t, tpl)
	ctx := t.Context()

	for _, key := range []string{"b1", "b2", "b3", "b4", "b5", "b6"} {
		if got, err := c.Bool(ctx, key, false); err != nil || !got {
			t.Errorf("Bool(%s) = %v, %v; want true, nil", key, got, err)
		}
	}

	for _, key := range []string{"b7", "b8", "b9", "b10"} {
		if got, err := c.Bool(ctx, key, true); err != nil || got {
			t.Errorf("Bool(%s) = %v, %v; want false, nil", key, got, err)
		}
	}
}

// TestEdgeJSONNilOut checks the dedicated nil-out sentinel.
func TestEdgeJSONNilOut(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)

	if err := c.JSON(t.Context(), "cfg", nil, nil); !errors.Is(err, ErrNilJSONOut) {
		t.Fatalf("JSON(nil out) = %v, want ErrNilJSONOut", err)
	}
}

// TestEdgeJSONIntoStruct checks decoding a stored JSON string into a typed
// struct.
func TestEdgeJSONIntoStruct(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)

	var out struct {
		A int `json:"a"`
	}

	if err := c.JSON(t.Context(), "cfg", &out, nil); err != nil {
		t.Fatalf("JSON(cfg) error = %v", err)
	}

	if out.A != 1 {
		t.Fatalf("out.A = %d, want 1", out.A)
	}
}

// TestEdgeConcurrentEvaluate exercises the goroutine-safe client under
// concurrent evaluation.
func TestEdgeConcurrentEvaluate(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)
	ctx := t.Context()

	var wg sync.WaitGroup

	for range 16 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 25 {
				if _, err := c.Bool(ctx, "new_ui", false); err != nil {
					t.Errorf("Bool() error = %v", err)
					return
				}

				if _, err := c.Int(ctx, "retry_count", 0); err != nil {
					t.Errorf("Int() error = %v", err)
					return
				}

				var out map[string]any
				if err := c.JSON(ctx, "cfg", &out, nil); err != nil {
					t.Errorf("JSON() error = %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()
}
