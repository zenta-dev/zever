package firebase

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"firebase.google.com/go/v4/remoteconfig"

	"github.com/zenta-dev/zever/flag"
)

const testTemplate = `{"parameters":{` +
	`"welcome":{"defaultValue":{"value":"hello"}},` +
	`"new_ui":{"defaultValue":{"value":"false"},"conditionalValues":{"is_pro":{"value":"true"}}},` +
	`"retry_count":{"defaultValue":{"value":"3"}},` +
	`"ratio":{"defaultValue":{"value":"3.9"}},` +
	`"neg":{"defaultValue":{"value":"-2.7"}},` +
	`"huge":{"defaultValue":{"value":"9999999999999999999999"}},` +
	`"bad_int":{"defaultValue":{"value":"abc"}},` +
	`"yes_word":{"defaultValue":{"value":"yes"}},` +
	`"maybe":{"defaultValue":{"value":"maybe"}},` +
	`"cfg":{"defaultValue":{"value":"{\"a\":1}"}},` +
	`"bad_json":{"defaultValue":{"value":"not-json"}}` +
	`},"conditions":[{"name":"is_pro","condition":{"customSignal":{"customSignalOperator":"STRING_EXACTLY_MATCHES","customSignalKey":"plan","targetCustomSignalValues":["pro"]}}}]}`

// newTestClient builds a client over a real ServerTemplate constructed
// hermetically: zero-value remoteconfig.Client needs no credentials and
// performs no I/O until Load, which tests never call (Set instead).
func newTestClient(t *testing.T) *client {
	t.Helper()
	tpl, err := (&remoteconfig.Client{}).InitServerTemplate(map[string]any{}, testTemplate)
	if err != nil {
		t.Fatalf("InitServerTemplate err = %v", err)
	}
	return &client{eval: tpl.Evaluate}
}

func proCtx() context.Context {
	return flag.WithEvalContext(context.Background(), flag.EvalContext{Signals: map[string]any{"plan": "pro"}})
}

func TestBool_defaultValue(t *testing.T) {
	t.Parallel()
	c := newTestClient(t)
	got, err := c.Bool(context.Background(), "new_ui", true)
	if err != nil {
		t.Fatalf("Bool err = %v", err)
	}
	if got {
		t.Error("Bool = true, want false (template default)")
	}
}

func TestBool_conditionalViaSignal(t *testing.T) {
	t.Parallel()
	c := newTestClient(t)
	got, err := c.Bool(proCtx(), "new_ui", false)
	if err != nil {
		t.Fatalf("Bool err = %v", err)
	}
	if !got {
		t.Error("Bool = false, want true (plan=pro condition)")
	}
}

func TestBool_sdkTruthinessSuperset(t *testing.T) {
	t.Parallel()
	c := newTestClient(t)
	ctx := context.Background()
	if got, err := c.Bool(ctx, "yes_word", false); err != nil || !got {
		t.Errorf("Bool(yes_word) = %v, %v; want true, nil (SDK truthy set)", got, err)
	}
	// "maybe" is not in the SDK truthy set: false with NO error.
	// Static adapter is stricter; firebase follows SDK semantics.
	if got, err := c.Bool(ctx, "maybe", true); err != nil || got {
		t.Errorf("Bool(maybe) = %v, %v; want false, nil", got, err)
	}
}

func TestMissing_returnsFallbackNilError(t *testing.T) {
	t.Parallel()
	c := newTestClient(t)
	ctx := context.Background()
	if got, err := c.Bool(ctx, "nope", true); err != nil || !got {
		t.Errorf("Bool missing = %v, %v; want true, nil", got, err)
	}
	if got, err := c.String(ctx, "nope", "fb"); err != nil || got != "fb" {
		t.Errorf("String missing = %q, %v; want %q, nil", got, err, "fb")
	}
	if got, err := c.Int(ctx, "nope", 42); err != nil || got != 42 {
		t.Errorf("Int missing = %v, %v; want 42, nil", got, err)
	}
	var out map[string]any
	if err := c.JSON(ctx, "nope", &out, map[string]any{"d": float64(1)}); err != nil {
		t.Errorf("JSON missing err = %v, want nil", err)
	} else if out["d"] != float64(1) {
		t.Errorf("JSON missing out = %v, want fallback fill", out)
	}
}

func TestString(t *testing.T) {
	t.Parallel()
	c := newTestClient(t)
	got, err := c.String(context.Background(), "welcome", "fb")
	if err != nil {
		t.Fatalf("String err = %v", err)
	}
	if got != "hello" {
		t.Errorf("String = %q, want %q", got, "hello")
	}
}

func TestInt_coerce(t *testing.T) {
	t.Parallel()
	c := newTestClient(t)
	ctx := context.Background()
	if got, err := c.Int(ctx, "retry_count", 0); err != nil || got != 3 {
		t.Errorf("Int(retry_count) = %v, %v; want 3, nil", got, err)
	}
	if got, err := c.Int(ctx, "ratio", 0); err != nil || got != 3 {
		t.Errorf("Int(ratio) = %v, %v; want 3 (trunc), nil", got, err)
	}
	if got, err := c.Int(ctx, "neg", 0); err != nil || got != -2 {
		t.Errorf("Int(neg) = %v, %v; want -2 (trunc toward zero), nil", got, err)
	}
}

func TestInt_unparseable_returnsFallbackError(t *testing.T) {
	t.Parallel()
	c := newTestClient(t)
	ctx := context.Background()
	for _, key := range []string{"bad_int", "huge"} {
		got, err := c.Int(ctx, key, 7)
		if err == nil {
			t.Errorf("Int(%s) err = nil, want non-nil", key)
		} else if !strings.Contains(err.Error(), "not an int") {
			t.Errorf("Int(%s) err = %q, want %q", key, err, "not an int")
		}
		if got != 7 {
			t.Errorf("Int(%s) = %v, want fallback 7", key, got)
		}
	}
}

func TestJSON_fill(t *testing.T) {
	t.Parallel()
	c := newTestClient(t)
	var out map[string]any
	if err := c.JSON(context.Background(), "cfg", &out, nil); err != nil {
		t.Fatalf("JSON err = %v", err)
	}
	if out["a"] != float64(1) {
		t.Errorf("JSON out = %v, want map[a:1]", out)
	}
}

func TestJSON_invalid_returnsError(t *testing.T) {
	t.Parallel()
	c := newTestClient(t)
	var out map[string]any
	if err := c.JSON(context.Background(), "bad_json", &out, nil); err == nil {
		t.Error("JSON(bad_json) err = nil, want non-nil")
	}
}

func TestJSON_nilOut_returnsError(t *testing.T) {
	t.Parallel()
	c := newTestClient(t)
	if err := c.JSON(context.Background(), "cfg", nil, nil); err == nil {
		t.Error("JSON nil out err = nil, want non-nil")
	}
}

func TestEvalError_wrapped(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("boom")
	c := &client{eval: func(map[string]any) (*remoteconfig.ServerConfig, error) { return nil, sentinel }}
	_, err := c.Bool(context.Background(), "welcome", false)
	if !errors.Is(err, sentinel) {
		t.Fatalf("Bool err = %v, want wrap of sentinel", err)
	}
	if !strings.Contains(err.Error(), "firebase: evaluate") {
		t.Errorf("Bool err %q missing %q", err.Error(), "firebase: evaluate")
	}
}

func TestContextCanceled_returnsFallbackError(t *testing.T) {
	t.Parallel()
	c := newTestClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Bool(ctx, "welcome", false); !errors.Is(err, context.Canceled) {
		t.Errorf("Bool canceled err = %v, want context.Canceled", err)
	}
	if _, err := c.String(ctx, "welcome", ""); !errors.Is(err, context.Canceled) {
		t.Errorf("String canceled err = %v, want context.Canceled", err)
	}
	if _, err := c.Int(ctx, "welcome", 0); !errors.Is(err, context.Canceled) {
		t.Errorf("Int canceled err = %v, want context.Canceled", err)
	}
	if err := c.JSON(ctx, "welcome", new(map[string]any), nil); !errors.Is(err, context.Canceled) {
		t.Errorf("JSON canceled err = %v, want context.Canceled", err)
	}
}

func TestInvalidKey_rejected(t *testing.T) {
	t.Parallel()
	c := newTestClient(t)
	ctx := context.Background()
	for _, key := range []string{"", "bad\nkey", strings.Repeat("k", 257)} {
		if _, err := c.Bool(ctx, key, false); !errors.Is(err, flag.ErrInvalidKey) {
			t.Errorf("Bool(%q) err = %v, want ErrInvalidKey", key, err)
		}
		if _, err := c.String(ctx, key, ""); !errors.Is(err, flag.ErrInvalidKey) {
			t.Errorf("String(%q) err = %v, want ErrInvalidKey", key, err)
		}
		if _, err := c.Int(ctx, key, 0); !errors.Is(err, flag.ErrInvalidKey) {
			t.Errorf("Int(%q) err = %v, want ErrInvalidKey", key, err)
		}
		if err := c.JSON(ctx, key, new(map[string]any), nil); !errors.Is(err, flag.ErrInvalidKey) {
			t.Errorf("JSON(%q) err = %v, want ErrInvalidKey", key, err)
		}
	}
}

func TestNew_validation(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sa := filepath.Join(dir, "sa.json")
	if err := os.WriteFile(sa, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := map[string]flag.Options{
		"empty project":      {Firebase: flag.FirebaseOptions{ProjectID: "", ServiceAccount: sa}},
		"empty sa":           {Firebase: flag.FirebaseOptions{ProjectID: "p", ServiceAccount: ""}},
		"negative timeout":   {Firebase: flag.FirebaseOptions{ProjectID: "p", ServiceAccount: sa, Timeout: -1}},
		"traversal":          {Firebase: flag.FirebaseOptions{ProjectID: "p", ServiceAccount: filepath.Join(dir, "..", "sa.json")}},
		"unclean":            {Firebase: flag.FirebaseOptions{ProjectID: "p", ServiceAccount: dir + "/./sa.json"}},
		"wrong ext":          {Firebase: flag.FirebaseOptions{ProjectID: "p", ServiceAccount: strings.TrimSuffix(sa, ".json")}},
		"directory":          {Firebase: flag.FirebaseOptions{ProjectID: "p", ServiceAccount: sub}},
		"missing but clean":  {Firebase: flag.FirebaseOptions{ProjectID: "p", ServiceAccount: filepath.Join(dir, "absent.json")}},
		"invalid json creds": {Firebase: flag.FirebaseOptions{ProjectID: "p", ServiceAccount: sa}},
	}
	for name, opts := range cases {
		if _, err := New(opts); err == nil {
			t.Errorf("New(%s) err = nil, want non-nil", name)
		}
	}
}

func TestNew_invalidOptionsType(t *testing.T) {
	t.Parallel()
	c := newTestClient(t)
	var _ flag.Flag = c
	if _, err := New(flag.Options{}); err == nil {
		t.Error("New(empty) err = nil, want non-nil")
	} else if !errors.Is(err, flag.ErrInvalidOptions) {
		t.Errorf("New(empty) err = %v, want ErrInvalidOptions", err)
	}
}

func TestClose_idempotentNil(t *testing.T) {
	t.Parallel()
	c := newTestClient(t)
	if err := c.Close(); err != nil {
		t.Errorf("Close err = %v, want nil", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close err = %v, want nil", err)
	}
}
