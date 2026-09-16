package parser

import (
	"net/http"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ast"
)

// parseSrc runs the parser over src end-to-end, as a caller would.
func parseSrc(t *testing.T, src string) (*ast.File, []string) {
	t.Helper()

	p := New("test.zen", []byte(src))
	file, errs := p.ParseFile()

	if file == nil {
		t.Fatalf("ParseFile returned a nil *ast.File for input:\n%s", src)
	}

	msgs := make([]string, len(errs))
	for i, d := range errs {
		msgs[i] = d.Error()
	}

	return file, msgs
}

// mustParseClean parses src and fails the test if any diagnostic was
// recorded, returning the resulting file otherwise.
func mustParseClean(t *testing.T, src string) *ast.File {
	t.Helper()

	file, msgs := parseSrc(t, src)
	if len(msgs) != 0 {
		t.Fatalf("expected zero diagnostics, got %d: %v\nsource:\n%s", len(msgs), msgs, src)
	}

	return file
}

func firstEntity(t *testing.T, file *ast.File) *ast.EntityDecl {
	t.Helper()

	for _, d := range file.Decls {
		if e, ok := d.(*ast.EntityDecl); ok {
			return e
		}
	}

	t.Fatalf("no EntityDecl found among %d top-level decls", len(file.Decls))

	return nil
}

func TestParseMinimalEntity(t *testing.T) {
	src := `entity User {
		id: uuid
	}`

	file := mustParseClean(t, src)

	if len(file.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(file.Decls))
	}

	e := firstEntity(t, file)

	if e.Name != "User" {
		t.Fatalf("entity name = %q, want %q", e.Name, "User")
	}

	if len(e.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(e.Fields))
	}

	f := e.Fields[0]
	if f.Name != "id" || f.Type == nil || f.Type.Name != "uuid" {
		t.Fatalf("field = %+v, want name=id type=uuid", f)
	}
}

func TestParseOptionalFieldMarker(t *testing.T) {
	src := `entity User {
		id: uuid
		email: string?
	}`

	file := mustParseClean(t, src)
	e := firstEntity(t, file)

	if len(e.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(e.Fields))
	}

	id := e.Fields[0]
	if id.Optional {
		t.Fatalf("field %q: Optional = true, want false (no trailing ?)", id.Name)
	}

	email := e.Fields[1]
	if email.Name != "email" || email.Type == nil || email.Type.Name != "string" {
		t.Fatalf("field = %+v, want name=email type=string", email)
	}

	if !email.Optional {
		t.Fatalf("field %q: Optional = false, want true (trailing ?)", email.Name)
	}
}

func TestParseOptionalFieldWithAttributesAndDefault(t *testing.T) {
	src := `entity User {
		nickname: string? @default("anon")
	}`

	file := mustParseClean(t, src)
	e := firstEntity(t, file)

	f := e.Fields[0]
	if !f.Optional {
		t.Fatalf("field %q: Optional = false, want true", f.Name)
	}

	if len(f.Attributes) != 1 || f.Attributes[0].Name != "default" {
		t.Fatalf("expected 1 default attribute, got %+v", f.Attributes)
	}
}

func TestParseStackedAttributes(t *testing.T) {
	src := `entity User {
		email: string @unique @validate(min_len: 1)
	}`

	file := mustParseClean(t, src)
	e := firstEntity(t, file)

	if len(e.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(e.Fields))
	}

	f := e.Fields[0]
	if len(f.Attributes) != 2 {
		t.Fatalf("expected 2 attributes, got %d: %+v", len(f.Attributes), f.Attributes)
	}

	if f.Attributes[0].Name != "unique" {
		t.Fatalf("attr[0].Name = %q, want unique", f.Attributes[0].Name)
	}

	if f.Attributes[1].Name != "validate" {
		t.Fatalf("attr[1].Name = %q, want validate", f.Attributes[1].Name)
	}

	if len(f.Attributes[1].Args) != 1 {
		t.Fatalf("expected 1 arg on validate, got %d", len(f.Attributes[1].Args))
	}

	arg := f.Attributes[1].Args[0]
	if arg.Name != "min_len" {
		t.Fatalf("arg.Name = %q, want min_len", arg.Name)
	}

	il, ok := arg.Value.(*ast.IntLit)
	if !ok || il.Value != 1 {
		t.Fatalf("arg.Value = %+v, want IntLit{1}", arg.Value)
	}
}

func TestParseNamedAndPositionalArgs(t *testing.T) {
	src := `entity User {
		id: uuid @bogus(1, 2, named: 3)
	}`

	file := mustParseClean(t, src)
	e := firstEntity(t, file)
	attr := e.Fields[0].Attributes[0]

	if len(attr.Args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(attr.Args))
	}

	if attr.Args[0].Name != "" || attr.Args[1].Name != "" {
		t.Fatalf("expected first two args positional, got names %q %q", attr.Args[0].Name, attr.Args[1].Name)
	}

	if attr.Args[2].Name != "named" {
		t.Fatalf("expected third arg named 'named', got %q", attr.Args[2].Name)
	}
}

func TestParseDefaultNowCall(t *testing.T) {
	src := `entity User {
		created_at: timestamp @default(now())
	}`

	file := mustParseClean(t, src)
	e := firstEntity(t, file)
	attr := e.Fields[0].Attributes[0]

	if attr.Name != "default" {
		t.Fatalf("attr.Name = %q, want default", attr.Name)
	}

	if len(attr.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(attr.Args))
	}

	call, ok := attr.Args[0].Value.(*ast.CallValue)
	if !ok {
		t.Fatalf("arg.Value = %T, want *ast.CallValue", attr.Args[0].Value)
	}

	if call.Name != "now" || len(call.Args) != 0 {
		t.Fatalf("call = %+v, want now() with zero args", call)
	}
}

func TestParseEnumFieldAndDefault(t *testing.T) {
	src := `entity User {
		status: enum(active, inactive) @default(active)
	}`

	file := mustParseClean(t, src)
	e := firstEntity(t, file)
	f := e.Fields[0]

	if f.Type.Name != "enum" {
		t.Fatalf("type name = %q, want enum", f.Type.Name)
	}

	if len(f.Type.Args) != 2 || f.Type.Args[0] != "active" || f.Type.Args[1] != "inactive" {
		t.Fatalf("type args = %v, want [active inactive]", f.Type.Args)
	}

	def := f.Attributes[0]

	ident, ok := def.Args[0].Value.(*ast.IdentValue)
	if !ok || ident.Name != "active" {
		t.Fatalf("default value = %+v, want IdentValue{active}", def.Args[0].Value)
	}
}

func TestParseEachRelationKind(t *testing.T) {
	cases := []struct {
		keyword string
		kind    ast.RelationKind
	}{
		{"has_many", ast.HasMany},
		{"has_one", ast.HasOne},
		{"belongs_to", ast.BelongsTo},
		{"many_to_many", ast.ManyToMany},
	}

	for _, tc := range cases {
		t.Run(tc.keyword, func(t *testing.T) {
			src := "entity Order {\n\t" + tc.keyword + " related: Target\n}"

			file := mustParseClean(t, src)
			e := firstEntity(t, file)

			if len(e.Relations) != 1 {
				t.Fatalf("expected 1 relation, got %d", len(e.Relations))
			}

			r := e.Relations[0]
			if r.Kind != tc.kind {
				t.Fatalf("relation kind = %v, want %v", r.Kind, tc.kind)
			}

			if r.FieldName != "related" || r.Target != "Target" {
				t.Fatalf("relation = %+v, want FieldName=related Target=Target", r)
			}
		})
	}
}

func TestParseManyToManyWithJoinTable(t *testing.T) {
	src := `entity Order {
		many_to_many tags: Tag {
			join_table: order_tags
		}
	}`

	file := mustParseClean(t, src)
	e := firstEntity(t, file)

	if len(e.Relations) != 1 {
		t.Fatalf("expected 1 relation, got %d", len(e.Relations))
	}

	r := e.Relations[0]
	if r.Join == nil {
		t.Fatalf("expected non-nil Join")
	}

	if r.Join.Table != "order_tags" {
		t.Fatalf("join table = %q, want order_tags", r.Join.Table)
	}
}

func TestParseIndexWithAndWithoutUnique(t *testing.T) {
	src := `entity Order {
		user_id: uuid
		index(user_id)
		index(user_id) @unique
	}`

	file := mustParseClean(t, src)
	e := firstEntity(t, file)

	if len(e.Indexes) != 2 {
		t.Fatalf("expected 2 indexes, got %d", len(e.Indexes))
	}

	if len(e.Indexes[0].Attributes) != 0 {
		t.Fatalf("expected first index to have no attributes, got %d", len(e.Indexes[0].Attributes))
	}

	if len(e.Indexes[1].Attributes) != 1 || e.Indexes[1].Attributes[0].Name != "unique" {
		t.Fatalf("expected second index to have @unique, got %+v", e.Indexes[1].Attributes)
	}

	if len(e.Indexes[1].Columns) != 1 || e.Indexes[1].Columns[0] != "user_id" {
		t.Fatalf("index columns = %v, want [user_id]", e.Indexes[1].Columns)
	}
}

func TestParseRPCWithHTTPAuthPermission(t *testing.T) {
	src := `service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			http: GET "/orders/{id}"
			auth: required(roles: {owner, admin})
			permission: check(resource: order)
		}
	}`

	file := mustParseClean(t, src)

	var svc *ast.ServiceDecl

	for _, d := range file.Decls {
		if s, ok := d.(*ast.ServiceDecl); ok {
			svc = s
		}
	}

	if svc == nil {
		t.Fatalf("no ServiceDecl found")
	}

	if len(svc.RPCs) != 1 {
		t.Fatalf("expected 1 rpc, got %d", len(svc.RPCs))
	}

	rpc := svc.RPCs[0]

	if rpc.Name != "GetOrder" || rpc.Returns != "Order" {
		t.Fatalf("rpc = %+v", rpc)
	}

	if len(rpc.Params) != 1 || rpc.Params[0].Name != "id" || rpc.Params[0].Type.Name != "uuid" {
		t.Fatalf("params = %+v", rpc.Params)
	}

	if rpc.HTTP == nil || rpc.HTTP.Method != http.MethodGet || rpc.HTTP.Path != "/orders/{id}" {
		t.Fatalf("http = %+v", rpc.HTTP)
	}

	authCall, ok := rpc.Auth.(*ast.CallValue)
	if !ok || authCall.Name != "required" {
		t.Fatalf("auth = %+v, want CallValue{required}", rpc.Auth)
	}

	permCall, ok := rpc.Permission.(*ast.CallValue)
	if !ok || permCall.Name != "check" {
		t.Fatalf("permission = %+v, want CallValue{check}", rpc.Permission)
	}
}

// TestParseRPCParamValidateAttribute proves @validate(...) parses on an RPC
// param, reusing the identical generic attribute-list grammar entity fields
// already have (see TestParseStackedAttributes) -- and that a param with no
// attributes still parses fine (existing entity-field @validate grammar
// unaffected regression check, via GetOrder's plain "id: uuid" param in the
// same rpc).
func TestParseRPCParamValidateAttribute(t *testing.T) {
	src := `service UserService {
		rpc CreateUser(id: uuid, email: string @validate(format: "email")) -> User {
			http: POST "/users"
		}
	}`

	file := mustParseClean(t, src)

	var svc *ast.ServiceDecl

	for _, d := range file.Decls {
		if s, ok := d.(*ast.ServiceDecl); ok {
			svc = s
		}
	}

	if svc == nil || len(svc.RPCs) != 1 {
		t.Fatalf("svc = %+v", svc)
	}

	rpc := svc.RPCs[0]

	if len(rpc.Params) != 2 {
		t.Fatalf("params = %+v, want 2", rpc.Params)
	}

	idParam := rpc.Params[0]
	if idParam.Name != "id" || len(idParam.Attributes) != 0 {
		t.Fatalf("id param = %+v, want no attributes", idParam)
	}

	emailParam := rpc.Params[1]
	if emailParam.Name != "email" || len(emailParam.Attributes) != 1 {
		t.Fatalf("email param = %+v, want exactly 1 attribute", emailParam)
	}

	attr := emailParam.Attributes[0]
	if attr.Name != "validate" || len(attr.Args) != 1 || attr.Args[0].Name != "format" {
		t.Fatalf("attribute = %+v, want @validate(format: ...)", attr)
	}

	lit, ok := attr.Args[0].Value.(*ast.StringLit)
	if !ok || lit.Value != "email" {
		t.Fatalf("format arg = %+v, want StringLit(\"email\")", attr.Args[0].Value)
	}
}

// TestParseRPCErrorsOption proves errors: {...} parses a mix of bare error
// codes (IdentValue) and codes with a positional string message
// (CallValue), in declaration order, alongside http/auth/permission on the
// same rpc.
func TestParseRPCErrorsOption(t *testing.T) {
	src := `service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			http: GET "/orders/{id}"
			auth: required
			permission: check(resource: order)
			errors: { not_found, invalid_argument("email is malformed") }
		}
	}`

	file := mustParseClean(t, src)

	svc, ok := file.Decls[0].(*ast.ServiceDecl)
	if !ok {
		t.Fatalf("decls[0] = %+v, want *ast.ServiceDecl", file.Decls[0])
	}

	rpc := svc.RPCs[0]

	if len(rpc.Errors) != 2 {
		t.Fatalf("expected 2 errors: items, got %d: %+v", len(rpc.Errors), rpc.Errors)
	}

	bare, ok := rpc.Errors[0].(*ast.IdentValue)
	if !ok || bare.Name != "not_found" {
		t.Fatalf("errors[0] = %+v, want IdentValue{not_found}", rpc.Errors[0])
	}

	withMsg, ok := rpc.Errors[1].(*ast.CallValue)
	if !ok || withMsg.Name != "invalid_argument" {
		t.Fatalf("errors[1] = %+v, want CallValue{invalid_argument}", rpc.Errors[1])
	}

	if len(withMsg.Args) != 1 {
		t.Fatalf("expected 1 arg on invalid_argument(...), got %d", len(withMsg.Args))
	}

	msg, ok := withMsg.Args[0].Value.(*ast.StringLit)
	if !ok || msg.Value != "email is malformed" {
		t.Fatalf("invalid_argument arg = %+v, want StringLit{email is malformed}", withMsg.Args[0].Value)
	}
}

// TestParseRPCErrorsOptionEmpty proves an empty errors: {} set parses
// cleanly (matching parseSetLit's existing permissive precedent), producing
// a non-nil-but-empty Errors slice check: len == 0.
func TestParseRPCErrorsOptionEmpty(t *testing.T) {
	src := `service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			errors: {}
		}
	}`

	file := mustParseClean(t, src)

	svc, ok := file.Decls[0].(*ast.ServiceDecl)
	if !ok {
		t.Fatalf("decls[0] = %+v, want *ast.ServiceDecl", file.Decls[0])
	}

	rpc := svc.RPCs[0]

	if len(rpc.Errors) != 0 {
		t.Fatalf("expected 0 errors: items, got %d: %+v", len(rpc.Errors), rpc.Errors)
	}
}

// TestParseRPCPaginatedTrue proves `paginated: true` parses on an rpc
// declaration alongside the existing http/auth options, unaffected by their
// grammar.
func TestParseRPCPaginatedTrue(t *testing.T) {
	src := `service TaskService {
		rpc ListTasks(user_id: uuid) -> Task {
			http: GET "/tasks"
			auth: required
			paginated: true
		}
	}`

	file := mustParseClean(t, src)

	svc, ok := file.Decls[0].(*ast.ServiceDecl)
	if !ok {
		t.Fatalf("decls[0] = %+v, want *ast.ServiceDecl", file.Decls[0])
	}

	rpc := svc.RPCs[0]

	if !rpc.PaginatedSet || !rpc.Paginated {
		t.Fatalf("rpc.Paginated = %v, PaginatedSet = %v, want true, true", rpc.Paginated, rpc.PaginatedSet)
	}

	if rpc.HTTP == nil || rpc.HTTP.Method != http.MethodGet {
		t.Fatalf("http option unaffected by paginated: true, got %+v", rpc.HTTP)
	}

	authCall, ok := rpc.Auth.(*ast.IdentValue)
	if !ok || authCall.Name != "required" {
		t.Fatalf("auth option unaffected by paginated: true, got %+v", rpc.Auth)
	}
}

// TestParseRPCPaginatedFalseExplicit proves an explicit `paginated: false`
// parses and is distinguishable, via PaginatedSet, from an absent option.
func TestParseRPCPaginatedFalseExplicit(t *testing.T) {
	src := `service TaskService {
		rpc GetTask(id: uuid) -> Task {
			paginated: false
		}
	}`

	file := mustParseClean(t, src)

	svc, ok := file.Decls[0].(*ast.ServiceDecl)
	if !ok {
		t.Fatalf("decls[0] = %+v, want *ast.ServiceDecl", file.Decls[0])
	}

	rpc := svc.RPCs[0]

	if !rpc.PaginatedSet {
		t.Fatalf("expected PaginatedSet = true for an explicit paginated: false")
	}

	if rpc.Paginated {
		t.Fatalf("expected Paginated = false, got true")
	}
}

// TestParseRPCPaginatedAbsentDefaultsFalse proves an rpc with no paginated:
// option at all resolves both Paginated and PaginatedSet to false -- the
// regression check that this feature is purely additive.
func TestParseRPCPaginatedAbsentDefaultsFalse(t *testing.T) {
	src := `service TaskService {
		rpc GetTask(id: uuid) -> Task {
			http: GET "/tasks/{id}"
		}
	}`

	file := mustParseClean(t, src)

	svc, ok := file.Decls[0].(*ast.ServiceDecl)
	if !ok {
		t.Fatalf("decls[0] = %+v, want *ast.ServiceDecl", file.Decls[0])
	}

	rpc := svc.RPCs[0]

	if rpc.Paginated || rpc.PaginatedSet {
		t.Fatalf("expected Paginated = false, PaginatedSet = false, got %v, %v", rpc.Paginated, rpc.PaginatedSet)
	}
}

func TestParseJobWithRetryBackoff(t *testing.T) {
	src := `job SendConfirmationEmail(order_id: uuid) {
		queue: emails
		retry: max_attempts(10), backoff(exponential, base: 30s)
	}`

	file := mustParseClean(t, src)

	var job *ast.JobDecl

	for _, d := range file.Decls {
		if j, ok := d.(*ast.JobDecl); ok {
			job = j
		}
	}

	if job == nil {
		t.Fatalf("no JobDecl found")
	}

	if job.Queue != "emails" {
		t.Fatalf("queue = %q, want emails", job.Queue)
	}

	if len(job.Params) != 1 || job.Params[0].Name != "order_id" {
		t.Fatalf("params = %+v", job.Params)
	}

	if len(job.Retry) != 2 {
		t.Fatalf("expected 2 retry values, got %d", len(job.Retry))
	}

	maxAttempts, ok := job.Retry[0].(*ast.CallValue)
	if !ok || maxAttempts.Name != "max_attempts" {
		t.Fatalf("retry[0] = %+v, want CallValue{max_attempts}", job.Retry[0])
	}

	backoff, ok := job.Retry[1].(*ast.CallValue)
	if !ok || backoff.Name != "backoff" {
		t.Fatalf("retry[1] = %+v, want CallValue{backoff}", job.Retry[1])
	}

	if len(backoff.Args) != 2 {
		t.Fatalf("backoff args = %+v, want 2 args", backoff.Args)
	}

	base, ok := backoff.Args[1].Value.(*ast.DurationLit)
	if !ok || !base.Valid || base.Raw != "30s" {
		t.Fatalf("backoff base = %+v, want valid DurationLit{30s}", backoff.Args[1].Value)
	}
}

func TestParseScheduleWithCronAndDispatch(t *testing.T) {
	src := `schedule DailyCleanup {
		cron: "0 0 * * *"
		dispatch: CleanupAbandonedCarts()
	}`

	file := mustParseClean(t, src)

	var sched *ast.ScheduleDecl

	for _, d := range file.Decls {
		if s, ok := d.(*ast.ScheduleDecl); ok {
			sched = s
		}
	}

	if sched == nil {
		t.Fatalf("no ScheduleDecl found")
	}

	if sched.Cron != "0 0 * * *" {
		t.Fatalf("cron = %q", sched.Cron)
	}

	call, ok := sched.Dispatch.(*ast.CallValue)
	if !ok || call.Name != "CleanupAbandonedCarts" {
		t.Fatalf("dispatch = %+v, want CallValue{CleanupAbandonedCarts}", sched.Dispatch)
	}
}

func TestParseNegativeNumberLiteral(t *testing.T) {
	src := `entity Product {
		discount: int64 @default(-5)
	}`

	file := mustParseClean(t, src)
	e := firstEntity(t, file)
	attr := e.Fields[0].Attributes[0]

	il, ok := attr.Args[0].Value.(*ast.IntLit)
	if !ok || il.Value != -5 {
		t.Fatalf("value = %+v, want IntLit{-5}", attr.Args[0].Value)
	}
}

func TestParseEntityLevelSchemaAttribute(t *testing.T) {
	src := `entity Order @schema(billing_archive) {
		id: uuid
	}`

	file := mustParseClean(t, src)
	e := firstEntity(t, file)

	if len(e.Attributes) != 1 || e.Attributes[0].Name != "schema" {
		t.Fatalf("entity attributes = %+v, want [schema(...)]", e.Attributes)
	}

	ident, ok := e.Attributes[0].Args[0].Value.(*ast.IdentValue)
	if !ok || ident.Name != "billing_archive" {
		t.Fatalf("schema arg = %+v, want IdentValue{billing_archive}", e.Attributes[0].Args[0].Value)
	}
}

func TestParseModuleWithOneOfEachDeclKind(t *testing.T) {
	// Module construct removed in v-next; this test now verifies that top-level
	// declarations for each remaining kind parse independently (dir-derived modules
	// are handled at the resolver layer, not via syntax).
	src := `entity Order {
		id: uuid
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			http: GET "/orders/{id}"
		}
	}

	job SendReceipt(order_id: uuid) {
		queue: emails
	}

	schedule DailyCleanup {
		cron: "0 0 * * *"
		dispatch: CleanupAbandonedCarts()
	}

	message CreateOrderRequest {
		user_id: uuid
		total_cents: int64
	}`

	file := mustParseClean(t, src)

	if len(file.Decls) != 5 {
		t.Fatalf("expected 5 top-level decls, got %d", len(file.Decls))
	}

	kinds := map[string]bool{}

	for _, d := range file.Decls {
		switch d.(type) {
		case *ast.EntityDecl:
			kinds["entity"] = true
		case *ast.ServiceDecl:
			kinds["service"] = true
		case *ast.JobDecl:
			kinds["job"] = true
		case *ast.ScheduleDecl:
			kinds["schedule"] = true
		case *ast.MessageDecl:
			kinds["message"] = true
		}
	}

	for _, want := range []string{"entity", "service", "job", "schedule", "message"} {
		if !kinds[want] {
			t.Fatalf("missing a %s decl, got kinds %v", want, kinds)
		}
	}
}

// TestParseWorkedExample compiles the whole worked example end-to-end (a
// User entity, an Order entity with relations and an index, an
// OrderService with two RPCs using http/auth, a job with retry/backoff, and
// a schedule with cron/dispatch) with zero diagnostics — the single
// highest-value positive regression test in this suite.
func TestParseWorkedExample(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string @unique @validate(min_len: 1)
		created_at: timestamp @default(now())
	}

	entity Order @schema(billing) {
		id: uuid @primary
		total_cents: int64
		status: enum(pending, paid, shipped, cancelled) @default(pending)

		belongs_to user: User
		has_many items: OrderItem
		many_to_many tags: Tag {
			join_table: order_tags
		}

		index(user_id)
		index(status) @unique
	}

	service OrderService {
		rpc GetOrder(id: uuid) -> Order {
			http: GET "/orders/{id}"
			auth: required(roles: {owner, admin})
		}

		rpc CreateOrder(user_id: uuid, total_cents: int64) -> Order {
			http: POST "/orders"
			auth: required(roles: {owner})
			permission: check(resource: order)
		}
	}

	job SendConfirmationEmail(order_id: uuid) {
		queue: emails
		retry: max_attempts(10), backoff(exponential, base: 30s)
	}

	schedule DailyCleanup {
		cron: "0 0 * * *"
		dispatch: CleanupAbandonedCarts()
	}`

	file := mustParseClean(t, src)

	if len(file.Decls) != 5 {
		t.Fatalf("expected 5 top-level decls, got %d", len(file.Decls))
	}

	wantKinds := []string{"*ast.EntityDecl", "*ast.EntityDecl", "*ast.ServiceDecl", "*ast.JobDecl", "*ast.ScheduleDecl"}

	for i, d := range file.Decls {
		got := goTypeName(d)
		if got != wantKinds[i] {
			t.Fatalf("decl[%d] = %s, want %s", i, got, wantKinds[i])
		}
	}

	order, ok := file.Decls[1].(*ast.EntityDecl)
	if !ok {
		t.Fatalf("decls[1] = %T, want *ast.EntityDecl", file.Decls[1])
	}

	if order.Name != "Order" || len(order.Attributes) != 1 || order.Attributes[0].Name != "schema" {
		t.Fatalf("order entity = %+v", order)
	}

	if len(order.Fields) != 3 || len(order.Relations) != 3 || len(order.Indexes) != 2 {
		t.Fatalf("order shape: fields=%d relations=%d indexes=%d", len(order.Fields), len(order.Relations), len(order.Indexes))
	}

	svc, ok := file.Decls[2].(*ast.ServiceDecl)
	if !ok || len(svc.RPCs) != 2 {
		t.Fatalf("service = %+v", svc)
	}
}

// goTypeName avoids importing "fmt" or "reflect" just to print a type name
// in a couple of assertions above.
func goTypeName(d ast.Decl) string {
	switch d.(type) {
	case *ast.EntityDecl:
		return "*ast.EntityDecl"
	case *ast.MessageDecl:
		return "*ast.MessageDecl"
	case *ast.ServiceDecl:
		return "*ast.ServiceDecl"
	case *ast.JobDecl:
		return "*ast.JobDecl"
	case *ast.ScheduleDecl:
		return "*ast.ScheduleDecl"
	default:
		return "unknown"
	}
}
