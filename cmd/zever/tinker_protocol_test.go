package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestTinkerProtocolRoundTrip marshals and unmarshals every request and
// result shape on the wire. The shim carries a duplicated copy of these
// declarations (it lives in another module), so their JSON encoding is the
// only thing holding the two sides together.
func TestTinkerProtocolRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		val  any
	}{
		{"tinkerRequest", tinkerRequest{Verb: verbDBQuery, Args: json.RawMessage(`{"sql":"select 1"}`)}},
		{"tinkerRequestNoArgs", tinkerRequest{Verb: verbPing}},
		{"tinkerResponseResult", tinkerResponse{Result: json.RawMessage(`{"value":true}`)}},
		{"tinkerResponseError", tinkerResponse{Error: "boom"}},
		{"tinkerResponseEmpty", tinkerResponse{}},
		{"dbQueryArgs", dbQueryArgs{SQL: "select ?", Args: []any{"x"}}},
		{"dbQueryArgsNoArgs", dbQueryArgs{SQL: "select 1"}},
		{"dbQueryResult", dbQueryResult{Columns: []string{"id", "title"}, Rows: [][]any{{"1", "a"}, {"2", "b"}}}},
		{"dbExecArgs", dbExecArgs{SQL: "delete from t where id = ?", Args: []any{"7"}}},
		{"cacheGetArgs", cacheGetArgs{Key: "k"}},
		{"cacheGetResult", cacheGetResult{Value: "v"}},
		{"cacheSetArgs", cacheSetArgs{Key: "k", Value: "v", TTLSeconds: 60}},
		{"cacheSetArgsNoTTL", cacheSetArgs{Key: "k", Value: "v"}},
		{"cacheDeleteArgs", cacheDeleteArgs{Key: "k"}},
		{"cacheExistsArgs", cacheExistsArgs{Key: "k"}},
		{"boolResult", boolResult{Value: true}},
		{"queuePushArgs", queuePushArgs{Topic: "t", Payload: "p", Headers: map[string]string{"a": "b"}}},
		{"queuePushArgsNoHeaders", queuePushArgs{Topic: "t", Payload: "p"}},
		{"queueLengthArgs", queueLengthArgs{Topic: "t"}},
		{"int64Result", int64Result{Value: 42}},
		{"jobDispatchArgs", jobDispatchArgs{Name: "send", Args: json.RawMessage(`{"to":"a@b.c"}`)}},
		{"jobDispatchArgsNoArgs", jobDispatchArgs{Name: "send"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tc.val)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}

			out := reflect.New(reflect.TypeOf(tc.val))
			if err := json.Unmarshal(data, out.Interface()); err != nil {
				t.Fatalf("unmarshal %s: %v", data, err)
			}

			got := out.Elem().Interface()
			if !reflect.DeepEqual(got, tc.val) {
				t.Fatalf("round trip mismatch via %s:\n got %#v\nwant %#v", data, got, tc.val)
			}
		})
	}
}

// TestTinkerVerbsAreUnique guards against a copy-paste collision in the verb
// constants: two verbs sharing a name would silently reroute a call.
func TestTinkerVerbsAreUnique(t *testing.T) {
	t.Parallel()

	verbs := []string{
		verbPing, verbDBQuery, verbDBExec,
		verbCacheGet, verbCacheSet, verbCacheDelete, verbCacheExists,
		verbQueuePush, verbQueueLength, verbJobDispatch,
	}

	seen := make(map[string]bool, len(verbs))

	for _, v := range verbs {
		if v == "" {
			t.Fatal("empty verb constant")
		}

		if seen[v] {
			t.Fatalf("duplicate verb %q", v)
		}

		seen[v] = true
	}
}

// TestTinkerFramePrefixIsNotJSON keeps the framing unambiguous: a prefix that
// could begin a JSON document would make raw shim output indistinguishable
// from protocol traffic.
func TestTinkerFramePrefixIsNotJSON(t *testing.T) {
	t.Parallel()

	if tinkerFramePrefix == "" {
		t.Fatal("frame prefix must not be empty")
	}

	if tinkerFramePrefix[0] == '{' || tinkerFramePrefix[0] == '[' {
		t.Fatalf("frame prefix %q must not start like JSON", tinkerFramePrefix)
	}
}
