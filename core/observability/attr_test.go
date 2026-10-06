package observability

import (
	"strings"
	"testing"
)

func TestAttr_constructors_carryKeyValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		attr  Attr
		key   string
		check func(t *testing.T, v AttributeValue)
	}{
		{name: "string", attr: String("k", "v"), key: "k", check: func(t *testing.T, v AttributeValue) {
			t.Helper()
			sv, ok := v.(StringValue)
			if !ok {
				t.Fatalf("value type = %T, want StringValue", v)
			}
			if sv.Value != "v" {
				t.Errorf("value = %q, want %q", sv.Value, "v")
			}
		}},
		{name: "int", attr: Int("k", 42), key: "k", check: func(t *testing.T, v AttributeValue) {
			t.Helper()
			iv, ok := v.(Int64Value)
			if !ok {
				t.Fatalf("value type = %T, want Int64Value", v)
			}
			if iv.Value != 42 {
				t.Errorf("value = %d, want 42", iv.Value)
			}
		}},
		{name: "int64", attr: Int64("k", -7), key: "k", check: func(t *testing.T, v AttributeValue) {
			t.Helper()
			iv, ok := v.(Int64Value)
			if !ok {
				t.Fatalf("value type = %T, want Int64Value", v)
			}
			if iv.Value != -7 {
				t.Errorf("value = %d, want -7", iv.Value)
			}
		}},
		{name: "float64", attr: Float64("k", 1.5), key: "k", check: func(t *testing.T, v AttributeValue) {
			t.Helper()
			fv, ok := v.(Float64Value)
			if !ok {
				t.Fatalf("value type = %T, want Float64Value", v)
			}
			if fv.Value != 1.5 {
				t.Errorf("value = %v, want 1.5", fv.Value)
			}
		}},
		{name: "bool", attr: Bool("k", true), key: "k", check: func(t *testing.T, v AttributeValue) {
			t.Helper()
			bv, ok := v.(BoolValue)
			if !ok {
				t.Fatalf("value type = %T, want BoolValue", v)
			}
			if bv.Value != true {
				t.Errorf("value = %v, want true", bv.Value)
			}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.attr.Key != tt.key {
				t.Errorf("Key = %q, want %q", tt.attr.Key, tt.key)
			}
			tt.check(t, tt.attr.Value)
		})
	}
}

func TestAttrValue_constructors_carryValue(t *testing.T) {
	t.Parallel()

	if got := StringAttr("v").Value; got != "v" {
		t.Errorf("StringAttr = %q, want %q", got, "v")
	}
	if got := IntAttr(3).Value; got != 3 {
		t.Errorf("IntAttr = %d, want 3", got)
	}
	if got := Int64Attr(-9).Value; got != -9 {
		t.Errorf("Int64Attr = %d, want -9", got)
	}
	if got := Float64Attr(2.5).Value; got != 2.5 {
		t.Errorf("Float64Attr = %v, want 2.5", got)
	}
	if got := BoolAttr(true).Value; got != true {
		t.Errorf("BoolAttr = %v, want true", got)
	}
}

func TestRedact_sensitiveKeys_redacted(t *testing.T) {
	t.Parallel()

	sensitive := []string{
		"password", "PASSWORD", "dbPassword",
		"secret", "Secret",
		"token", "TOKEN",
		"api_key", "API_KEY", "apikey", "APIKEY",
		"auth", "Auth",
		"cookie", "session",
		"private_key", "PRIVATE_KEY", "privatekey",
	}
	for _, key := range sensitive {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			if !redact(key) {
				t.Errorf("redact(%q) = false, want true", key)
			}
		})
	}
}

func TestRedact_benignKeys_kept(t *testing.T) {
	t.Parallel()

	benign := []string{"user", "service", "endpoint", "trace_id", "count"}
	for _, key := range benign {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			if redact(key) {
				t.Errorf("redact(%q) = true, want false", key)
			}
		})
	}
}

func TestNormalizeAttrs_redact_replacesValue(t *testing.T) {
	t.Parallel()

	got := normalizeAttrs([]Attr{String("password", "hunter2")}, 0)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	sv, ok := got[0].Value.(StringValue)
	if !ok {
		t.Fatalf("value type = %T, want StringValue", got[0].Value)
	}
	if sv.Value != "[redacted]" {
		t.Errorf("value = %q, want %q", sv.Value, "[redacted]")
	}
}

// TestSemconvKeys pins the exported OpenTelemetry semantic-convention
// keys. The literal values are the contract with dashboards and saved
// queries, so a rename here is a breaking change and must fail the build.
func TestSemconvKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "http request method", got: HTTPRequestMethod, want: "http.request.method"},
		{name: "url path", got: URLPath, want: "url.path"},
		{name: "http response status code", got: HTTPResponseStatusCode, want: "http.response.status_code"},
		{name: "server address", got: ServerAddress, want: "server.address"},
		{name: "rpc system", got: RPCSystem, want: "rpc.system"},
		{name: "rpc service", got: RPCService, want: "rpc.service"},
		{name: "rpc method", got: RPCMethod, want: "rpc.method"},
		{name: "rpc grpc status code", got: RPCGRPCStatusCode, want: "rpc.grpc.status_code"},
		{name: "messaging system", got: MessagingSystem, want: "messaging.system"},
		{name: "messaging destination name", got: MessagingDestinationName, want: "messaging.destination.name"},
		{name: "messaging operation", got: MessagingOperation, want: "messaging.operation"},
		{name: "messaging message id", got: MessagingMessageID, want: "messaging.message.id"},
		{name: "messaging message conversation id", got: MessagingMessageConversationID, want: "messaging.message.conversation_id"},
		{name: "grpc system value", got: SystemGRPC, want: "grpc"},
		{name: "publish operation", got: OperationPublish, want: "publish"},
		{name: "process operation", got: OperationProcess, want: "process"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Errorf("key = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

// TestSemconvKeys_notRedacted pins that normalizeAttrs keeps every
// semconv key verbatim: none of them may trip the secret-key heuristic
// and be replaced with "[redacted]".
func TestSemconvKeys_notRedacted(t *testing.T) {
	t.Parallel()

	keys := []string{
		HTTPRequestMethod, URLPath, HTTPResponseStatusCode, ServerAddress,
		RPCSystem, RPCService, RPCMethod, RPCGRPCStatusCode,
		MessagingSystem, MessagingDestinationName, MessagingOperation,
		MessagingMessageID, MessagingMessageConversationID,
	}

	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			got := normalizeAttrs([]Attr{String(key, "value")}, 0)
			if len(got) != 1 {
				t.Fatalf("len = %d, want 1", len(got))
			}
			sv, ok := got[0].Value.(StringValue)
			if !ok {
				t.Fatalf("value type = %T, want StringValue", got[0].Value)
			}
			if sv.Value != "value" {
				t.Errorf("value = %q, want value (semconv key must not be redacted)", sv.Value)
			}
		})
	}
}

func TestNormalizeAttrs_truncate_clipsLongStrings(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", MaxValueLen+100)
	got := normalizeAttrs([]Attr{String("k", long)}, 0)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	sv, ok := got[0].Value.(StringValue)
	if !ok {
		t.Fatalf("value type = %T, want StringValue", got[0].Value)
	}
	if len(sv.Value) != MaxValueLen {
		t.Errorf("len = %d, want %d", len(sv.Value), MaxValueLen)
	}
}

func TestNormalizeAttrs_truncate_respectsCustomLimit(t *testing.T) {
	t.Parallel()

	got := normalizeAttrs([]Attr{String("k", "abcdef")}, 3)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	sv, ok := got[0].Value.(StringValue)
	if !ok {
		t.Fatalf("value type = %T, want StringValue", got[0].Value)
	}
	if sv.Value != "abc" {
		t.Errorf("value = %q, want %q", sv.Value, "abc")
	}
}

func TestNormalizeAttrs_cap_dropsBeyondMaxAttrs(t *testing.T) {
	t.Parallel()

	in := make([]Attr, 0, MaxAttrs+5)
	for i := 0; i < MaxAttrs+5; i++ {
		in = append(in, Int("k", i))
	}
	got := normalizeAttrs(in, 0)
	if len(got) != MaxAttrs {
		t.Errorf("len = %d, want %d", len(got), MaxAttrs)
	}
}
