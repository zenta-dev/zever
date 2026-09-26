package proto

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
	"github.com/zenta-dev/zever/dsl/resolver"
)

var update = flag.Bool("update", false, "update golden files")

// compileSchema runs a .zen source string through the parser and resolver,
// failing the test on any diagnostic from either stage.
func compileSchema(t *testing.T, src string) *ir.Schema {
	t.Helper()

	p := parser.New("test.zen", []byte(src))

	file, diags := p.ParseFile()
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	return schema
}

func compileMultiModule(t *testing.T, files map[string]string) *ir.Schema {
	t.Helper()

	astFiles := make([]*ast.File, 0, len(files))
	for name, src := range files {
		p := parser.New(name, []byte(src))
		f, diags := p.ParseFile()
		if diags.HasErrors() {
			t.Fatalf("parse errors in %s: %v", name, diags)
		}
		astFiles = append(astFiles, f)
	}
	schema, diags := resolver.ResolveWithSchemaDir(astFiles, "schema")
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}
	return schema
}

// checkGolden compares got against testdata/golden/<name>.proto.golden,
// rewriting the golden file instead when -update is passed.
func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()

	path := filepath.Join("testdata", "golden", name+".proto.golden")

	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}

		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}

	if string(got) != string(want) {
		t.Fatalf("golden mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// readTestdata reads a vendored googleapis fixture relative to
// testdata/googleapis.
func readTestdata(t *testing.T, rel string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", "googleapis", rel))
	if err != nil {
		t.Fatalf("read testdata %s: %v", rel, err)
	}

	return string(b)
}

// validateProto compiles content (as the file at path) with the protoc
// toolchain, with zever/annotations.proto, the two vendored google/api/*.proto
// fixtures, and the well-known types (from /usr/include when present) all
// available as imports. It fails the test on any compile error, catching
// "matches a stale golden file but is actually broken proto" bugs that a byte
// comparison alone would miss. It shells out to protoc via the standard
// library only, so this package gains no new module dependencies; when protoc
// is not installed the validation is skipped rather than failed.
// (Source's helper used protocompile here; same property, stdlib only.)
func validateProto(t *testing.T, path string, content []byte) {
	t.Helper()

	protoc, err := exec.LookPath("protoc")
	if err != nil {
		t.Skip("protoc not installed; skipping proto compile validation")
	}

	dir := t.TempDir()

	files := map[string]string{
		path:                           string(content),
		"zever/annotations.proto":      AnnotationsProtoSource,
		"google/api/http.proto":        readTestdata(t, filepath.Join("google", "api", "http.proto")),
		"google/api/annotations.proto": readTestdata(t, filepath.Join("google", "api", "annotations.proto")),
	}

	for name, src := range files {
		full := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("create dirs for %s: %v", name, err)
		}

		if err := os.WriteFile(full, []byte(src), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	args := []string{"--proto_path=" + dir}
	if _, err := os.Stat("/usr/include/google/protobuf/descriptor.proto"); err == nil {
		args = append(args, "--proto_path=/usr/include")
	}

	args = append(args, "--descriptor_set_out="+filepath.Join(dir, "out.pb"), "--include_imports", filepath.FromSlash(path))

	if combined, err := exec.CommandContext(t.Context(), protoc, args...).CombinedOutput(); err != nil {
		t.Fatalf("protoc validation of %s failed: %v\n%s", path, err, combined)
	}
}

func TestGenerateSimpleEntity(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string @unique
		created_at: timestamp @default(now())
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	checkGolden(t, "simple_entity", content)
	validateProto(t, "schema.proto", content)
}

// TestGenerateOptionalFieldEmitsOptionalKeyword proves a field declared
// optional (`field: type?`) emits proto3's explicit "optional" field
// modifier, while a non-optional field does not. protoc validation
// (validateProto) proves the emitted "optional" keyword is itself valid
// proto3, not just a string match.
func TestGenerateOptionalFieldEmitsOptionalKeyword(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string?
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	validateProto(t, "schema.proto", content)

	got := string(content)

	if !strings.Contains(got, "optional string email = 2;") {
		t.Fatalf("expected optional field emission, got:\n%s", got)
	}

	if strings.Contains(got, "optional string id") || strings.Contains(got, "optional string uuid") {
		t.Fatalf("non-optional field unexpectedly marked optional:\n%s", got)
	}

	if !strings.Contains(got, "string id = 1;") {
		t.Fatalf("non-optional id field should render without the optional keyword:\n%s", got)
	}
}

// TestGenerateScalarMapping hits every row of the field-type mapping table
// on one entity (uuid, string, int32, int64, float32, float64, bool,
// timestamp, date, bytes, json, enum) plus a belongs_to and a has_many
// relation, proving relations are excluded from the rendered message
// entirely.
func TestGenerateScalarMapping(t *testing.T) {
	src := `entity Owner {
		id: uuid @primary
	}

	entity Part {
		id: uuid @primary
		widget_id: uuid
	}

	entity Widget {
		id: uuid @primary
		owner_id: uuid
		name: string
		quantity: int32
		views: int64
		ratio: float32
		price: float64
		active: bool
		created_at: timestamp
		valid_on: date
		payload: bytes
		metadata: json
		status: enum(pending, active, archived)

		belongs_to owner: Owner @foreign_key(owner_id)
		has_many parts: Part @foreign_key(widget_id)
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	checkGolden(t, "scalar_mapping", content)
	validateProto(t, "schema.proto", content)

	rendered := string(content)
	if strings.Contains(rendered, "owner") && strings.Contains(rendered, "Owner owner") {
		t.Fatalf("relation field leaked into rendered message:\n%s", rendered)
	}

	if strings.Contains(rendered, "repeated Part") {
		t.Fatalf("has_many relation leaked into rendered message as a repeated field:\n%s", rendered)
	}
}

// TestGenerateServiceWithOptions covers a service with GET/POST/DELETE http
// bindings, an auth option with roles, an auth option with no roles, and a
// permission option.
func TestGenerateServiceWithOptions(t *testing.T) {
	src := `entity Order {
		id: uuid @primary
		user_id: uuid
		amount_cents: int64
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			http: GET "/v1/orders/{id}"
			auth: required(roles: {owner, admin})
		}

		rpc CreateOrder(user_id: uuid, amount_cents: int64) -> Order {
			http: POST "/v1/orders"
			auth: required(roles: {owner})
			permission: check("owns_order", resource: Order, owner_field: user_id)
		}

		rpc DeleteOrder(id: uuid) -> Order {
			http: DELETE "/v1/orders/{id}"
			auth: required
		}
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	checkGolden(t, "service_with_options", content)
	validateProto(t, "schema.proto", content)
}

// TestGeneratePaginatedOperation proves a `paginated: true` rpc gets a
// synthesized "<Name>Response" message (repeated <Entity> items + string
// next_cursor) used as its rpc's response type, alongside a non-paginated
// rpc in the same service whose response type is unaffected (still the bare
// entity message name) -- both the paginated-shape and regression checks the
// plan calls for.
func TestGeneratePaginatedOperation(t *testing.T) {
	src := `entity Task {
		id: uuid @primary
		title: string
	}

	service TaskService {
		rpc ListTasks(user_id: uuid) -> Task {
			http: GET "/v1/tasks"
			paginated: true
			auth: required
		}

		rpc GetTask(id: uuid) -> Task {
			http: GET "/v1/tasks/{id}"
			auth: required
		}
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	checkGolden(t, "paginated_operation", content)
	validateProto(t, "schema.proto", content)

	rendered := string(content)

	if !strings.Contains(rendered, "message ListTasksResponse {\n  repeated Task items = 1;\n  string next_cursor = 2;\n}") {
		t.Fatalf("missing synthesized ListTasksResponse message:\n%s", rendered)
	}

	if !strings.Contains(rendered, "rpc ListTasks(ListTasksRequest) returns (ListTasksResponse)") {
		t.Fatalf("ListTasks rpc does not return the synthesized ListTasksResponse:\n%s", rendered)
	}

	if !strings.Contains(rendered, "rpc GetTask(GetTaskRequest) returns (Task)") {
		t.Fatalf("non-paginated GetTask rpc unexpectedly changed:\n%s", rendered)
	}

	if strings.Contains(rendered, "GetTaskResponse") {
		t.Fatalf("a non-paginated operation must not get a synthesized response message:\n%s", rendered)
	}
}

// TestGenerateErrorsOption covers a bare error code and a code with a
// message on the same rpc, proving the rendered
// zever.annotations.v1.errors option string is correct and that it
// validates as real proto against the extension's own message shape.
func TestGenerateErrorsOption(t *testing.T) {
	src := `entity Order {
		id: uuid @primary
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			auth: required
			errors: { not_found, invalid_argument("email is malformed") }
		}
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	checkGolden(t, "errors_option", content)
	validateProto(t, "schema.proto", content)

	rendered := string(content)
	want := `option (zever.annotations.v1.errors) = ` +
		`{ cases: [{ code: "NOT_FOUND" }, { code: "INVALID_ARGUMENT", message: "email is malformed" }] };`

	if !strings.Contains(rendered, want) {
		t.Fatalf("rendered schema.proto missing expected errors option.\nwant substring: %s\ngot:\n%s", want, rendered)
	}
}

// TestGenerateErrorsOptionOnlyOptionStillImportsAnnotations proves the
// zever/annotations.proto import fires even when errors: is the only
// *option-rendering* declaration on an rpc besides the now-mandatory
// auth:/permission: (secure-by-default), matching auth/permission's own
// existing import-triggering behavior. http: is deliberately absent here.
func TestGenerateErrorsOptionOnlyOptionStillImportsAnnotations(t *testing.T) {
	src := `entity Order {
		id: uuid @primary
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			auth: required
			errors: { not_found }
		}
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	rendered := string(content)
	if !strings.Contains(rendered, `import "zever/annotations.proto";`) {
		t.Fatalf("expected zever/annotations.proto import when errors: is the only option, got:\n%s", rendered)
	}

	validateProto(t, "schema.proto", content)
}

// TestGenerateErrorsCombinedWithHTTPAuthPermission is the end-to-end test
// combining errors: with http:/auth:/permission: on the same rpc, proving
// the zever/annotations.proto import line appears exactly once in the
// rendered file regardless of how many of the four options triggered it.
func TestGenerateErrorsCombinedWithHTTPAuthPermission(t *testing.T) {
	src := `entity Order {
		id: uuid @primary
		user_id: uuid
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			http: GET "/v1/orders/{id}"
			auth: required(roles: {owner, admin})
			permission: check("owns_order", resource: Order, owner_field: user_id)
			errors: { not_found, permission_denied("caller does not own this order") }
		}
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	checkGolden(t, "errors_with_http_auth_permission", content)
	validateProto(t, "schema.proto", content)

	rendered := string(content)

	n := strings.Count(rendered, `import "zever/annotations.proto";`)
	if n != 1 {
		t.Fatalf("expected exactly 1 zever/annotations.proto import line, got %d:\n%s", n, rendered)
	}

	for _, want := range []string{
		`option (google.api.http) = { get: "/v1/orders/{id}" };`,
		`option (zever.annotations.v1.auth) = { required: true, roles: ["owner", "admin"] };`,
		`option (zever.annotations.v1.permission) = { check: "owns_order", resource: "Order", owner_field: "user_id" };`,
		`option (zever.annotations.v1.errors) = ` +
			`{ cases: [{ code: "NOT_FOUND" }, { code: "PERMISSION_DENIED", message: "caller does not own this order" }] };`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered schema.proto missing expected option.\nwant substring: %s\ngot:\n%s", want, rendered)
		}
	}
}

// TestGenerateTwoModules proves the per-module file split needs no
// cross-file import: each module's own service returns only that module's
// own entity, so neither rendered file's text references the other
// module's path.
func TestGenerateTwoModules(t *testing.T) {
	schema := compileMultiModule(t, map[string]string{
		"schema/billing/billing.zen": `entity Invoice {
			id: uuid @primary
			amount_cents: int64
		}

		service BillingService {
			rpc GetInvoice(id: uuid) -> Invoice {
				http: GET "/v1/invoices/{id}"
				auth: required
			}
		}`,
		"schema/shipping/shipping.zen": `entity Shipment {
			id: uuid @primary
			tracking_code: string
		}

		service ShippingService {
			rpc GetShipment(id: uuid) -> Shipment {
				http: GET "/v1/shipments/{id}"
				auth: required
			}
		}`,
	})

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	wantKeys := []string{"billing/schema.proto", "shipping/schema.proto", "zever/annotations.proto"}

	if len(out) != len(wantKeys) {
		t.Fatalf("Generate output keys = %v, want exactly %v", keys(out), wantKeys)
	}

	for _, k := range wantKeys {
		if _, ok := out[k]; !ok {
			t.Fatalf("Generate output missing %q, got keys %v", k, keys(out))
		}
	}

	billing := out["billing/schema.proto"]
	shipping := out["shipping/schema.proto"]

	checkGolden(t, "two_module_billing", billing)
	checkGolden(t, "two_module_shipping", shipping)

	validateProto(t, "billing/schema.proto", billing)
	validateProto(t, "shipping/schema.proto", shipping)

	if strings.Contains(string(billing), "shipping/schema.proto") {
		t.Fatalf("billing/schema.proto imports shipping's file:\n%s", billing)
	}

	if strings.Contains(string(shipping), "billing/schema.proto") {
		t.Fatalf("shipping/schema.proto imports billing's file:\n%s", shipping)
	}
}

// TestGenerateVersionedModulePackageName is the regression case a dir-derived
// schema/v1/<module>/*.zen layout used to get wrong: module.Name used to BE
// the version segment itself ("v1"), producing the doubled-up package
// "zever.v1.v1". Now Version is derived separately, so a real module name
// survives and the version feeds moduleNaming instead of a hardcoded "v1".
func TestGenerateVersionedModulePackageName(t *testing.T) {
	schema := compileMultiModule(t, map[string]string{
		"schema/v1/iam/user.zen": `entity User {
			id: uuid @primary
		}`,
	})

	if len(schema.Modules) != 1 || schema.Modules[0].Name != "iam" || schema.Modules[0].Version != "v1" {
		t.Fatalf("resolved module = %+v, want Name=iam Version=v1", schema.Modules[0])
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	iam, ok := out["iam/schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing %q, got keys %v", "iam/schema.proto", keys(out))
	}

	if !strings.Contains(string(iam), "package zever.iam.v1;") {
		t.Fatalf("expected package zever.iam.v1, got:\n%s", iam)
	}

	if strings.Contains(string(iam), "zever.v1.v1") {
		t.Fatalf("regression: package name doubled up the version:\n%s", iam)
	}
}

// TestGenerateUnversionedModuleDefaultsToV1 confirms an unversioned schema
// (no version segment in its path) keeps generating exactly what it always
// has: moduleNaming defaults an empty Version to "v1".
func TestGenerateUnversionedModuleDefaultsToV1(t *testing.T) {
	schema := compileMultiModule(t, map[string]string{
		"schema/billing/order.zen": `entity Order {
			id: uuid @primary
		}`,
	})

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	billing, ok := out["billing/schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing %q, got keys %v", "billing/schema.proto", keys(out))
	}

	if !strings.Contains(string(billing), "package zever.billing.v1;") {
		t.Fatalf("expected package zever.billing.v1, got:\n%s", billing)
	}
}

// TestGenerateWorkedExample runs the design doc's full worked example (a
// User entity; an Order entity with belongs_to/has_many/many_to_many
// relations, an enum field, and indexes; an OrderService with two RPCs
// using http/auth/permission; a job with retry/backoff; and a schedule with
// cron/dispatch) end-to-end with zero diagnostics, and validates the
// generated output through protoc.
func TestGenerateWorkedExample(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string @unique @validate(min_len: 1)
		created_at: timestamp @default(now())
	}

	entity Order @schema(billing) {
		id: uuid @primary
		user_id: uuid
		total_cents: int64
		status: enum(pending, paid, shipped, cancelled) @default(pending)

		belongs_to user: User @foreign_key(user_id)
		has_many items: OrderItem @foreign_key(order_id)
		many_to_many tags: Tag {
			join_table: order_tags
		}

		index(user_id)
		index(status) @unique
	}

	entity OrderItem {
		id: uuid @primary
		order_id: uuid
		sku: string
	}

	entity Tag {
		id: uuid @primary
		name: string @unique

		many_to_many orders: Order {
			join_table: order_tags
		}
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			http: GET "/orders/{id}"
			auth: required(roles: {owner, admin})
		}

		rpc CreateOrder(user_id: uuid, total_cents: int64) -> Order {
			http: POST "/orders"
			auth: required(roles: {owner})
			permission: check("owns_order", resource: Order, owner_field: user_id)
		}
	}

	job SendConfirmationEmail(order_id: uuid) {
		queue: emails
		retry: max_attempts(10), backoff(exponential, base: 30s)
	}

	job CleanupAbandonedCarts() {
	}

	schedule DailyCleanup {
		cron: "0 0 * * *"
		dispatch: CleanupAbandonedCarts()
	}`

	p := parser.New("worked_example.zen", []byte(src))

	file, diags := p.ParseFile()
	if len(diags) != 0 {
		t.Fatalf("expected zero parse diagnostics, got %d: %v", len(diags), diags)
	}

	schema, diags := resolver.Resolve([]*ast.File{file})
	if len(diags) != 0 {
		t.Fatalf("expected zero resolve diagnostics, got %d: %v", len(diags), diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	checkGolden(t, "worked_example", content)
	validateProto(t, "schema.proto", content)
}

// TestGenerateEmptySchema covers a schema with zero entities/services (the
// implicit module still exists, per the resolver's own invariant): the
// rendered file must still be valid, import-free proto.
func TestGenerateEmptySchema(t *testing.T) {
	schema := compileSchema(t, "")

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	checkGolden(t, "empty_schema", content)
	validateProto(t, "schema.proto", content)

	if strings.Contains(string(content), "import") {
		t.Fatalf("empty schema.proto should be import-free:\n%s", content)
	}
}

// TestGenerateRPCNameCollision proves that two services in the same module
// both declaring an RPC named "Get" is reported as a real Generate error
// naming both colliding RPCs, instead of silently emitting two "message
// GetRequest { ... }" blocks into one proto file (invalid proto, previously
// produced with zero diagnostics).
func TestGenerateRPCNameCollision(t *testing.T) {
	src := `entity Order {
		id: uuid @primary
	}

	entity User {
		id: uuid @primary
	}

	service OrderService {
		rpc Get(id: uuid) -> Order { auth: required }
	}

	service UserService {
		rpc Get(id: uuid) -> User { auth: required }
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err == nil {
		t.Fatalf("Generate: expected a collision error, got nil (output keys %v)", keys(out))
	}

	msg := err.Error()
	if !strings.Contains(msg, "OrderService.Get") || !strings.Contains(msg, "UserService.Get") {
		t.Fatalf("Generate error %q does not name both colliding RPCs", msg)
	}
}

// TestGenerateRPCNameNoCollision proves that two services with distinct RPC
// names in the same module still compile with zero errors — the fix for
// TestGenerateRPCNameCollision must not produce false positives.
func TestGenerateRPCNameNoCollision(t *testing.T) {
	src := `entity Order {
		id: uuid @primary
	}

	entity User {
		id: uuid @primary
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order { auth: required }
	}

	service UserService {
		rpc GetUser(id: uuid) -> User { auth: required }
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: unexpected error for non-colliding RPC names: %v", err)
	}

	if _, ok := out["schema.proto"]; !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}
}

// TestGenerateEnumValueCollision proves that two enum values that normalize
// to the same SCREAMING_SNAKE token (e.g. "inProgress" and "in_progress"
// both becoming "IN_PROGRESS") is reported as a real Generate error, instead
// of silently emitting two identical enum value names in one nested enum.
func TestGenerateEnumValueCollision(t *testing.T) {
	src := `entity Task {
		id: uuid @primary
		status: enum(inProgress, in_progress)
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err == nil {
		t.Fatalf("Generate: expected an enum-value collision error, got nil (output keys %v)", keys(out))
	}

	msg := err.Error()
	if !strings.Contains(msg, "IN_PROGRESS") {
		t.Fatalf("Generate error %q does not name the colliding enum value", msg)
	}
}

// TestGenerateMessageFieldRef proves a message field that references
// another message by bare name (ir.Field.Ref, resolved by
// resolver_message.go's resolveFieldForMessage) renders as a proto field
// embedding that other message's own generated type directly, via the same
// renderRefField already used for ref-typed RPC params -- not as some
// unresolved/invalid scalar. TokenPairResponse is used only as an RPC
// Returns type (never a param), proving this rendering path is independent
// of validate-everything-in / request-position handling.
func TestGenerateMessageFieldRef(t *testing.T) {
	src := `entity User {
		id: uuid @primary
	}

	message TokenResponse {
		value: string @validate(min_len: 1)
		expires_at: timestamp
	}

	message TokenPairResponse {
		access: TokenResponse
		refresh: TokenResponse
	}

	service AuthService {
		rpc Login(email: string @validate(format: "email")) -> TokenPairResponse {
			auth: required
		}
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	got := string(content)

	if !strings.Contains(got, "TokenResponse access = 1;") {
		t.Fatalf("expected TokenPairResponse.access to render as a TokenResponse ref field, got:\n%s", got)
	}

	if !strings.Contains(got, "TokenResponse refresh = 2;") {
		t.Fatalf("expected TokenPairResponse.refresh to render as a TokenResponse ref field, got:\n%s", got)
	}

	validateProto(t, "schema.proto", content)
}

// TestNewAnnotationsProtoUnchanged proves New()'s zever/annotations.proto
// output is byte-for-byte identical to the raw embedded
// AnnotationsProtoSource -- New() must not rewrite anything, since every
// existing test/golden file compiles against the default go_package root.
func TestNewAnnotationsProtoUnchanged(t *testing.T) {
	src := `entity Order {
		id: uuid @primary
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			auth: required()
		}
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["zever/annotations.proto"]
	if !ok {
		t.Fatalf("Generate output missing zever/annotations.proto, got keys %v", keys(out))
	}

	if string(content) != AnnotationsProtoSource {
		t.Fatalf("New().Generate's zever/annotations.proto differs from the raw embedded source:\n%s", content)
	}
}

// TestNewWithAnnotationsGoPackageRootRewritesOnlyGoPackageLine proves a
// Backend constructed via NewWithAnnotationsGoPackageRoot rewrites only the
// companion file's own "option go_package" line -- rewriting its import
// root while leaving "annotationsv1" as the Go package name and every other
// line of the file (the auth/permission proto extensions it defines)
// untouched.
func TestNewWithAnnotationsGoPackageRootRewritesOnlyGoPackageLine(t *testing.T) {
	const overrideRoot = "github.com/acme/api/generated/protogogen/zever/annotations"

	src := `entity Order {
		id: uuid @primary
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			auth: required()
		}
	}`

	schema := compileSchema(t, src)

	out, err := NewWithAnnotationsGoPackageRoot(overrideRoot).Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["zever/annotations.proto"]
	if !ok {
		t.Fatalf("Generate output missing zever/annotations.proto, got keys %v", keys(out))
	}

	rendered := string(content)

	wantLine := `option go_package = "` + overrideRoot + `;annotationsv1";`
	if !strings.Contains(rendered, wantLine) {
		t.Fatalf("expected rewritten go_package line %q, got:\n%s", wantLine, rendered)
	}

	if strings.Contains(rendered, defaultAnnotationsGoPackageRoot) {
		t.Fatalf("rendered zever/annotations.proto still references the default go_package root:\n%s", rendered)
	}

	// Every other line must be identical to the raw embedded source: diff
	// line-by-line, skipping only the one go_package line.
	wantLines := strings.Split(AnnotationsProtoSource, "\n")
	gotLines := strings.Split(rendered, "\n")

	if len(wantLines) != len(gotLines) {
		t.Fatalf("rewritten file has %d lines, want %d (line count must not change)", len(gotLines), len(wantLines))
	}

	for i, wantLine := range wantLines {
		if strings.HasPrefix(wantLine, "option go_package") {
			continue
		}

		if gotLines[i] != wantLine {
			t.Fatalf("line %d differs from embedded source.\nwant: %q\ngot:  %q", i+1, wantLine, gotLines[i])
		}
	}
}

// TestNewWithAnnotationsGoPackageRootDoesNotAffectProtoImportLines proves
// the override only changes the companion file's own go_package Go import
// path, never how OTHER rendered files reference it via proto's
// import-by-.proto-path mechanism (which is a file path, not a Go import
// path, and is therefore unaffected by this override by construction) --
// verified explicitly rather than assumed.
func TestNewWithAnnotationsGoPackageRootDoesNotAffectProtoImportLines(t *testing.T) {
	src := `entity Order {
		id: uuid @primary
		user_id: uuid
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			http: GET "/v1/orders/{id}"
			auth: required(roles: {owner, admin})
			permission: check("owns_order", resource: Order, owner_field: user_id)
		}
	}`

	schema := compileSchema(t, src)

	out, err := NewWithAnnotationsGoPackageRoot("github.com/acme/api/generated/protogogen/zever/annotations").
		Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	moduleContent, ok := out["schema.proto"]
	if !ok {
		t.Fatalf("Generate output missing schema.proto, got keys %v", keys(out))
	}

	rendered := string(moduleContent)

	if !strings.Contains(rendered, `import "zever/annotations.proto";`) {
		t.Fatalf("expected schema.proto's own import line to reference zever/annotations.proto by .proto path unchanged, got:\n%s", rendered)
	}

	if strings.Contains(rendered, "github.com/acme") {
		t.Fatalf("schema.proto unexpectedly references the overridden Go import root:\n%s", rendered)
	}

	validateProto(t, "schema.proto", moduleContent)
}

// TestNewWithAnnotationsGoPackageRootEmptyFallsBackToDefault proves an
// empty override string falls back to defaultAnnotationsGoPackageRoot,
// matching NewWithPBImportRoot's equivalent gogen behavior.
func TestNewWithAnnotationsGoPackageRootEmptyFallsBackToDefault(t *testing.T) {
	src := `entity Order {
		id: uuid @primary
	}`

	schema := compileSchema(t, src)

	out, err := NewWithAnnotationsGoPackageRoot("").Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["zever/annotations.proto"]
	if !ok {
		t.Fatalf("Generate output missing zever/annotations.proto, got keys %v", keys(out))
	}

	if string(content) != AnnotationsProtoSource {
		t.Fatalf("empty override should fall back to the default go_package root, got:\n%s", content)
	}
}

// keys returns a sorted slice of a map's keys, for deterministic test
// failure messages.
func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}
