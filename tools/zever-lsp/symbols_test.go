package main

import (
	"testing"

	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/compile"
	"github.com/zenta-dev/zever/dsl/ir"
	"github.com/zenta-dev/zever/dsl/parser"
)

const symbolsSrc = `entity User {
	id: uuid @primary
	email: string @unique
}

entity Task {
	id: uuid @primary
	user_id: uuid
	title: string

	belongs_to user: User @foreign_key(user_id)
}

service TaskService {
	rpc ListTasks(user_id: uuid) -> Task {
		http: GET "/tasks"
		auth: required
	}
}

job SendDigest() {
	queue: low
}

schedule NightlyDigest {
	cron: "0 0 * * *"
	dispatch: SendDigest()
}
`

// findSymbol returns the top-level symbol with the given name.
func findSymbol(symbols protocol.DocumentSymbolSlice, name string) *protocol.DocumentSymbol {
	for i := range symbols {
		if symbols[i].Name == name {
			return &symbols[i]
		}
	}

	return nil
}

func childNames(symbol *protocol.DocumentSymbol) []string {
	names := make([]string, 0, len(symbol.Children))
	for _, child := range symbol.Children {
		names = append(names, child.Name)
	}

	return names
}

func TestSymbolsFromSchema(t *testing.T) {
	result, diags := compile.Compile(map[string]string{"main.zen": symbolsSrc})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	symbols := symbolsFromSchema(result.Schema, "main.zen")

	if len(symbols) != 5 {
		t.Fatalf("got %d top-level symbols, want 5: %v", len(symbols), symbols)
	}

	tests := []struct {
		name string
		kind protocol.SymbolKind
	}{
		{"User", protocol.SymbolKindClass},
		{"Task", protocol.SymbolKindClass},
		{"TaskService", protocol.SymbolKindInterface},
		{"SendDigest", protocol.SymbolKindFunction},
		{"NightlyDigest", protocol.SymbolKindEvent},
	}

	for _, tc := range tests {
		symbol := findSymbol(symbols, tc.name)
		if symbol == nil {
			t.Errorf("symbol %q not found", tc.name)
			continue
		}

		if symbol.Kind != tc.kind {
			t.Errorf("symbol %q kind = %d, want %d", tc.name, symbol.Kind, tc.kind)
		}
	}

	task := findSymbol(symbols, "Task")
	if task == nil {
		t.Fatal("Task symbol missing")
	}

	// Three fields, then the relation, all as SymbolKindField children.
	wantChildren := []string{"id", "user_id", "title", "user"}

	got := childNames(task)
	if len(got) != len(wantChildren) {
		t.Fatalf("Task children = %v, want %v", got, wantChildren)
	}

	for i, want := range wantChildren {
		if got[i] != want {
			t.Errorf("Task child %d = %q, want %q", i, got[i], want)
		}

		if task.Children[i].Kind != protocol.SymbolKindField {
			t.Errorf("Task child %q kind = %d, want %d", got[i], task.Children[i].Kind, protocol.SymbolKindField)
		}
	}

	// The relation child's detail names the resolved target entity.
	relation := task.Children[3]
	if relation.Detail == nil || *relation.Detail != "belongs_to User" {
		t.Errorf("relation detail = %v, want \"belongs_to User\"", relation.Detail)
	}

	svc := findSymbol(symbols, "TaskService")
	if svc == nil {
		t.Fatal("TaskService symbol missing")
	}

	if len(svc.Children) != 1 || svc.Children[0].Name != "ListTasks" {
		t.Fatalf("TaskService children = %v, want [ListTasks]", childNames(svc))
	}

	if svc.Children[0].Kind != protocol.SymbolKindMethod {
		t.Errorf("ListTasks kind = %d, want %d", svc.Children[0].Kind, protocol.SymbolKindMethod)
	}

	rpc := svc.Children[0]
	if got := childNames(&rpc); len(got) != 1 || got[0] != "user_id" {
		t.Errorf("ListTasks children = %v, want [user_id]", got)
	}

	if len(rpc.Children) == 1 {
		param := rpc.Children[0]
		if param.Kind != symbolKindParameter {
			t.Errorf("user_id kind = %d, want parameter kind %d", param.Kind, symbolKindParameter)
		}

		if param.Detail == nil || *param.Detail != "uuid" {
			t.Errorf("user_id detail = %v, want \"uuid\"", param.Detail)
		}
	}
}

// TestSymbolsFromSchemaFiltersByFile checks that only symbols declared in the
// requested document are returned, even though the schema merges every file.
func TestSymbolsFromSchemaFiltersByFile(t *testing.T) {
	result, diags := compile.Compile(map[string]string{
		"a.zen": "entity User {\n\tid: uuid @primary\n}\n",
		"b.zen": "entity Task {\n\tid: uuid @primary\n}\n",
		"c.zen": "service Svc {\n\trpc Get(id: uuid) -> Task {\n\t\tauth: none\n\t}\n}\n\n" +
			"job SendDigest() {\n\tqueue: low\n}\n\n" +
			"schedule NightlyDigest {\n\tcron: \"0 0 * * *\"\n\tdispatch: SendDigest()\n}\n",
	})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	symbols := symbolsFromSchema(result.Schema, "a.zen")

	if len(symbols) != 1 || symbols[0].Name != "User" {
		t.Fatalf("a.zen symbols = %v, want just User", symbols)
	}

	// The other direction: a file holding a service, a job, and a schedule
	// reports exactly those three, proving every declaration-kind loop
	// honors the file filter.
	other := symbolsFromSchema(result.Schema, "c.zen")

	if len(other) != 3 {
		names := make([]string, 0, len(other))
		for _, sym := range other {
			names = append(names, sym.Name)
		}

		t.Fatalf("c.zen symbols = %v, want 3 (service, job, schedule)", names)
	}
}

// TestSymbolsFromASTFallback covers the mid-edit path: resolution fails, but
// the parser still recovers enough structure for a useful outline.
func TestSymbolsFromASTFallback(t *testing.T) {
	// `nonexistent_type` resolves to nothing, so whole-workspace resolution
	// errors and the server falls back to the raw AST.
	broken := "entity User {\n\tid: uuid @primary\n\tweird: nonexistent_type\n}\n"

	_, diags := compile.Compile(map[string]string{"broken.zen": broken})
	if !diags.HasErrors() {
		t.Fatal("expected the sample to fail resolution, so the fallback path is exercised")
	}

	file, _ := parser.New("broken.zen", []byte(broken)).ParseFile()

	symbols := symbolsFromAST(file)

	if len(symbols) != 1 {
		t.Fatalf("got %d symbols, want 1: %v", len(symbols), symbols)
	}

	if symbols[0].Name != "User" || symbols[0].Kind != protocol.SymbolKindClass {
		t.Errorf("symbol = %q kind %d, want User/class", symbols[0].Name, symbols[0].Kind)
	}

	got := childNames(&symbols[0])
	if len(got) != 2 || got[0] != "id" || got[1] != "weird" {
		t.Errorf("children = %v, want [id weird]", got)
	}
}

// TestSymbolsFromSchemaNilReturnsNil pins the nil-schema guard: no resolved
// schema means no document symbols, not a panic.
func TestSymbolsFromSchemaNilReturnsNil(t *testing.T) {
	if got := symbolsFromSchema(nil, "main.zen"); got != nil {
		t.Errorf("symbolsFromSchema(nil, ...) = %v, want nil", got)
	}
}

// findWorkspaceSymbol returns the first symbol with the given name.
func findWorkspaceSymbol(symbols protocol.SymbolInformationSlice, name string) *protocol.SymbolInformation {
	for i := range symbols {
		if symbols[i].Name == name {
			return &symbols[i]
		}
	}

	return nil
}

func workspaceSymbolNames(symbols protocol.SymbolInformationSlice) []string {
	names := make([]string, 0, len(symbols))
	for _, sym := range symbols {
		names = append(names, sym.Name)
	}

	return names
}

const workspaceSymbolsUserSrc = `entity User {
	id: uuid @primary
	email: string @unique
}

service UserService {
	rpc GetUser(id: uuid) -> User {
		http: GET "/users/{id}"
		auth: required
	}
}
`

const workspaceSymbolsOrderSrc = `entity Order {
	id: uuid @primary
	amount: int64
}
`

// TestWorkspaceSymbolsQueryFiltersAcrossFiles proves the query is a
// case-insensitive substring match spanning every file in the workspace,
// and that unrelated symbols are excluded.
func TestWorkspaceSymbolsQueryFiltersAcrossFiles(t *testing.T) {
	files := map[string]string{
		"user.zen":  workspaceSymbolsUserSrc,
		"order.zen": workspaceSymbolsOrderSrc,
	}

	result, diags := compile.Compile(files)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	symbols := workspaceSymbols(result.Schema, files, "User")

	if findWorkspaceSymbol(symbols, "User") == nil {
		t.Errorf("expected User entity in results: %v", workspaceSymbolNames(symbols))
	}

	if findWorkspaceSymbol(symbols, "UserService") == nil {
		t.Errorf("expected UserService in results: %v", workspaceSymbolNames(symbols))
	}

	if findWorkspaceSymbol(symbols, "Order") != nil {
		t.Errorf("did not expect unrelated Order entity in results: %v", workspaceSymbolNames(symbols))
	}

	// Lower-case query proves the match is case-insensitive.
	lower := workspaceSymbols(result.Schema, files, "user")
	if findWorkspaceSymbol(lower, "User") == nil {
		t.Errorf("case-insensitive query missed User: %v", workspaceSymbolNames(lower))
	}
}

// TestWorkspaceSymbolsEmptyQueryReturnsEverything checks the spec-required
// behavior: an empty query returns every symbol across every file.
func TestWorkspaceSymbolsEmptyQueryReturnsEverything(t *testing.T) {
	files := map[string]string{
		"user.zen":  workspaceSymbolsUserSrc,
		"order.zen": workspaceSymbolsOrderSrc,
	}

	result, diags := compile.Compile(files)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	symbols := workspaceSymbols(result.Schema, files, "")

	for _, name := range []string{"User", "UserService", "Order"} {
		if findWorkspaceSymbol(symbols, name) == nil {
			t.Errorf("expected %q in unfiltered results: %v", name, workspaceSymbolNames(symbols))
		}
	}
}

// TestWorkspaceSymbolsIncludesFieldsAndRPCs proves the flat list is not
// shallow: field- and RPC-level symbols are present, not just top-level
// entity/service declarations.
func TestWorkspaceSymbolsIncludesFieldsAndRPCs(t *testing.T) {
	files := map[string]string{"user.zen": workspaceSymbolsUserSrc}

	result, diags := compile.Compile(files)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	symbols := workspaceSymbols(result.Schema, files, "")

	email := findWorkspaceSymbol(symbols, "email")
	if email == nil {
		t.Fatalf("expected field symbol %q, got: %v", "email", workspaceSymbolNames(symbols))
	}

	if email.Kind != protocol.SymbolKindField {
		t.Errorf("email kind = %d, want %d", email.Kind, protocol.SymbolKindField)
	}

	if email.ContainerName == nil || *email.ContainerName != "User" {
		t.Errorf("email containerName = %v, want User", email.ContainerName)
	}

	rpc := findWorkspaceSymbol(symbols, "GetUser")
	if rpc == nil {
		t.Fatalf("expected RPC symbol %q, got: %v", "GetUser", workspaceSymbolNames(symbols))
	}

	if rpc.Kind != protocol.SymbolKindMethod {
		t.Errorf("GetUser kind = %d, want %d", rpc.Kind, protocol.SymbolKindMethod)
	}

	if rpc.ContainerName == nil || *rpc.ContainerName != "UserService" {
		t.Errorf("GetUser containerName = %v, want UserService", rpc.ContainerName)
	}
}

// TestWorkspaceSymbolsFallbackToAST covers the mid-edit path: one file in
// the workspace fails to resolve, but symbols recovered from the AST of the
// files that DO parse are still returned.
func TestWorkspaceSymbolsFallbackToAST(t *testing.T) {
	broken := "entity Broken {\n\tid: uuid @primary\n\tweird: nonexistent_type\n}\n"

	files := map[string]string{
		"user.zen":   workspaceSymbolsUserSrc,
		"broken.zen": broken,
	}

	_, diags := compile.Compile(files)
	if !diags.HasErrors() {
		t.Fatal("expected the sample to fail resolution, so the fallback path is exercised")
	}

	// Mirrors how the server calls workspaceSymbols once Recompile's result
	// is deemed unusable: schema is nil, forcing the per-file AST fallback.
	symbols := workspaceSymbols(nil, files, "")

	if findWorkspaceSymbol(symbols, "User") == nil {
		t.Errorf("expected User recovered via AST fallback: %v", workspaceSymbolNames(symbols))
	}

	if findWorkspaceSymbol(symbols, "UserService") == nil {
		t.Errorf("expected UserService recovered via AST fallback: %v", workspaceSymbolNames(symbols))
	}

	if findWorkspaceSymbol(symbols, "email") == nil {
		t.Errorf("expected field email recovered via AST fallback: %v", workspaceSymbolNames(symbols))
	}
}

func TestFieldTypeName(t *testing.T) {
	tests := []struct {
		name string
		in   ir.FieldType
		want string
	}{
		{"scalar", ir.FieldType{Scalar: ir.TUUID}, "uuid"},
		{"string", ir.FieldType{Scalar: ir.TString}, "string"},
		{"enum with values", ir.FieldType{Scalar: ir.TEnum, EnumValues: []string{"a", "b"}}, "enum(a, b)"},
		{"enum without values", ir.FieldType{Scalar: ir.TEnum}, "enum"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := fieldTypeName(tc.in); got != tc.want {
				t.Errorf("fieldTypeName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestScalarName_unknownType_returnsUnknown(t *testing.T) {
	if got := scalarName(ir.ScalarType(999)); got != "unknown" {
		t.Errorf("scalarName(999) = %q, want %q", got, "unknown")
	}
}

func TestRelationKindName_allKeywords_rendered(t *testing.T) {
	tests := []struct {
		name string
		in   ir.RelationKind
		want string
	}{
		{"has many", ir.HasMany, "has_many"},
		{"has one", ir.HasOne, "has_one"},
		{"belongs to", ir.BelongsTo, "belongs_to"},
		{"many to many", ir.ManyToMany, "many_to_many"},
		{"unknown", ir.RelationKind(999), "relation"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := relationKindName(tc.in); got != tc.want {
				t.Errorf("relationKindName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAstRelationKindName_allKeywords_rendered(t *testing.T) {
	tests := []struct {
		name string
		in   ast.RelationKind
		want string
	}{
		{"has many", ast.HasMany, "has_many"},
		{"has one", ast.HasOne, "has_one"},
		{"belongs to", ast.BelongsTo, "belongs_to"},
		{"many to many", ast.ManyToMany, "many_to_many"},
		{"unknown", ast.RelationKind(999), "relation"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := astRelationKindName(tc.in); got != tc.want {
				t.Errorf("astRelationKindName() = %q, want %q", got, tc.want)
			}
		})
	}
}

// astAllKindsSrc exercises every top-level declaration kind the AST
// fallback must recover: an entity with a relation, a service with an RPC,
// a job, and a schedule.
const astAllKindsSrc = `entity Task {
	id: uuid @primary
	user_id: uuid

	belongs_to user: User @foreign_key(user_id)
}

service TaskService {
	rpc ListTasks(user_id: uuid) -> Task {
		http: GET "/tasks"
		auth: required
	}
}

job SendDigest() {
	queue: low
}

schedule NightlyDigest {
	cron: "0 0 * * *"
	dispatch: SendDigest()
}
`

// TestSymbolsFromASTFallbackAllDeclKinds proves the fallback outline is not
// entity-only: relations nest under their entity, RPCs under their service,
// and jobs/schedules appear as top-level symbols.
func TestSymbolsFromASTFallbackAllDeclKinds(t *testing.T) {
	file, _ := parser.New("all.zen", []byte(astAllKindsSrc)).ParseFile()
	if file == nil {
		t.Fatal("failed to parse fixture")
	}

	symbols := symbolsFromAST(file)

	if len(symbols) != 4 {
		t.Fatalf("got %d symbols, want 4: %v", len(symbols), symbols)
	}

	task := findSymbol(symbols, "Task")
	if task == nil {
		t.Fatal("Task symbol missing")
	}

	got := childNames(task)
	if len(got) != 3 || got[0] != "id" || got[1] != "user_id" || got[2] != "user" {
		t.Errorf("Task children = %v, want [id user_id user]", got)
	}

	relation := task.Children[2]
	if relation.Detail == nil || *relation.Detail != "belongs_to User" {
		t.Errorf("relation detail = %v, want \"belongs_to User\"", relation.Detail)
	}

	svc := findSymbol(symbols, "TaskService")
	if svc == nil {
		t.Fatal("TaskService symbol missing")
	}

	if len(svc.Children) != 1 || svc.Children[0].Name != "ListTasks" {
		t.Fatalf("TaskService children = %v, want [ListTasks]", childNames(svc))
	}

	if svc.Children[0].Detail == nil || *svc.Children[0].Detail != "-> Task" {
		t.Errorf("ListTasks detail = %v, want \"-> Task\"", svc.Children[0].Detail)
	}

	rpc := svc.Children[0]
	if got := childNames(&rpc); len(got) != 1 || got[0] != "user_id" {
		t.Errorf("ListTasks children = %v, want [user_id]", got)
	}

	if len(rpc.Children) == 1 {
		param := rpc.Children[0]
		if param.Kind != symbolKindParameter {
			t.Errorf("user_id kind = %d, want parameter kind %d", param.Kind, symbolKindParameter)
		}

		if param.Detail == nil || *param.Detail != "uuid" {
			t.Errorf("user_id detail = %v, want \"uuid\"", param.Detail)
		}
	}

	if job := findSymbol(symbols, "SendDigest"); job == nil || job.Kind != protocol.SymbolKindFunction {
		t.Errorf("SendDigest job symbol missing or wrong kind: %v", job)
	}

	if sched := findSymbol(symbols, "NightlyDigest"); sched == nil || sched.Kind != protocol.SymbolKindEvent {
		t.Errorf("NightlyDigest schedule symbol missing or wrong kind: %v", sched)
	}
}

// TestFlatSymbolsFromASTAllDeclKinds proves the workspace fallback flattens
// every declaration kind with the right symbol kind and container linkage.
func TestFlatSymbolsFromASTAllDeclKinds(t *testing.T) {
	file, _ := parser.New("all.zen", []byte(astAllKindsSrc)).ParseFile()
	if file == nil {
		t.Fatal("failed to parse fixture")
	}

	symbols := flatSymbolsFromAST("all.zen", file)
	names := workspaceSymbolNames(symbols)

	for _, want := range []string{"Task", "id", "user", "TaskService", "ListTasks", "SendDigest", "NightlyDigest"} {
		if findWorkspaceSymbol(symbols, want) == nil {
			t.Errorf("expected %q in flat AST symbols: %v", want, names)
		}
	}

	rpc := findWorkspaceSymbol(symbols, "ListTasks")
	if rpc == nil {
		t.Fatal("ListTasks symbol missing")
	}

	if rpc.Kind != protocol.SymbolKindMethod {
		t.Errorf("ListTasks kind = %d, want %d", rpc.Kind, protocol.SymbolKindMethod)
	}

	if rpc.ContainerName == nil || *rpc.ContainerName != "TaskService" {
		t.Errorf("ListTasks containerName = %v, want TaskService", rpc.ContainerName)
	}

	field := findWorkspaceSymbol(symbols, "user")
	if field == nil {
		t.Fatal("user relation symbol missing")
	}

	if field.ContainerName == nil || *field.ContainerName != "Task" {
		t.Errorf("user containerName = %v, want Task", field.ContainerName)
	}
}

func TestFlatSymbolsFromSchemaNilReturnsNil(t *testing.T) {
	if got := flatSymbolsFromSchema(nil); got != nil {
		t.Errorf("flatSymbolsFromSchema(nil) = %v, want nil", got)
	}
}

// TestWorkspaceSymbolsFullSchemaIncludesJobsSchedulesAndRelations proves the
// resolved workspace list reaches every declaration kind, including the
// relation field, the job, and the schedule symbolsSrc declares.
func TestWorkspaceSymbolsFullSchemaIncludesJobsSchedulesAndRelations(t *testing.T) {
	files := map[string]string{"main.zen": symbolsSrc}

	result, diags := compile.Compile(files)
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	symbols := workspaceSymbols(result.Schema, files, "")

	job := findWorkspaceSymbol(symbols, "SendDigest")
	if job == nil || job.Kind != protocol.SymbolKindFunction {
		t.Errorf("SendDigest job symbol missing or wrong kind: %v", workspaceSymbolNames(symbols))
	}

	sched := findWorkspaceSymbol(symbols, "NightlyDigest")
	if sched == nil || sched.Kind != protocol.SymbolKindEvent {
		t.Errorf("NightlyDigest schedule symbol missing or wrong kind: %v", workspaceSymbolNames(symbols))
	}

	rel := findWorkspaceSymbol(symbols, "user")
	if rel == nil {
		t.Fatalf("user relation symbol missing: %v", workspaceSymbolNames(symbols))
	}

	if rel.Kind != protocol.SymbolKindField {
		t.Errorf("user kind = %d, want %d", rel.Kind, protocol.SymbolKindField)
	}

	if rel.ContainerName == nil || *rel.ContainerName != "Task" {
		t.Errorf("user containerName = %v, want Task", rel.ContainerName)
	}
}

// symbolsVisibilitySrc adds the three previously invisible declaration kinds
// -- a named enum, a message, and a saga -- to the resolved-schema surface.
const symbolsVisibilitySrc = `enum Status { active, dormant }

entity User {
	id: uuid @primary
	status: Status
}

message CreateUserInput {
	email: string @validate(format: "email")
}

service UserService {
	rpc CreateUser(input: CreateUserInput) -> User {
		http: POST "/users"
		auth: required
	}

	rpc Charge(id: uuid) -> User {
		http: POST "/users/{id}/charge"
		auth: required
	}

	rpc Refund(id: uuid) -> User {
		http: POST "/users/{id}/refund"
		auth: required
	}
}

saga Checkout {
	step reserve {
		execute: UserService.CreateUser
		compensate: UserService.Refund
	}
	step charge {
		execute: UserService.Charge
		pivot: true
	}
}
`

func compileSymbolsVisibilityFixture(t *testing.T) protocol.DocumentSymbolSlice {
	t.Helper()

	result, diags := compile.Compile(map[string]string{"main.zen": symbolsVisibilitySrc})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	return symbolsFromSchema(result.Schema, "main.zen")
}

// TestSymbolsFromSchemaIncludesMessagesSagasEnums proves the outline now
// covers the message (with field children), the saga (with step children)
// and the named enum under the resolved-schema path.
func TestSymbolsFromSchemaIncludesMessagesSagasEnums(t *testing.T) {
	symbols := compileSymbolsVisibilityFixture(t)

	msg := findSymbol(symbols, "CreateUserInput")
	if msg == nil {
		t.Fatal("CreateUserInput symbol missing")
	}

	if msg.Kind != protocol.SymbolKindStruct {
		t.Errorf("CreateUserInput kind = %d, want %d", msg.Kind, protocol.SymbolKindStruct)
	}

	if got := childNames(msg); len(got) != 1 || got[0] != "email" {
		t.Errorf("message children = %v, want [email]", got)
	}

	saga := findSymbol(symbols, "Checkout")
	if saga == nil {
		t.Fatal("Checkout symbol missing")
	}

	if saga.Kind != protocol.SymbolKindEvent {
		t.Errorf("Checkout kind = %d, want %d", saga.Kind, protocol.SymbolKindEvent)
	}

	if got := childNames(saga); len(got) != 2 || got[0] != "reserve" || got[1] != "charge" {
		t.Errorf("saga children = %v, want [reserve charge]", got)
	}

	enum := findSymbol(symbols, "Status")
	if enum == nil {
		t.Fatal("Status symbol missing")
	}

	if enum.Kind != protocol.SymbolKindEnum {
		t.Errorf("Status kind = %d, want %d", enum.Kind, protocol.SymbolKindEnum)
	}
}

// TestSagaStepSymbolDetailCarriesReferences proves a step's outline detail
// names its forward and reverse references, so the RPC pair survives
// collapse without opening the file.
func TestSagaStepSymbolDetailCarriesReferences(t *testing.T) {
	symbols := compileSymbolsVisibilityFixture(t)

	saga := findSymbol(symbols, "Checkout")
	if saga == nil {
		t.Fatal("Checkout symbol missing")
	}

	reserve := saga.Children[0]
	if reserve.Detail == nil || *reserve.Detail != "execute UserService.CreateUser, compensate UserService.Refund" {
		t.Errorf("reserve detail = %v, want the execute/compensate pair", reserve.Detail)
	}

	charge := saga.Children[1]
	if charge.Detail == nil || *charge.Detail != "execute UserService.Charge (pivot)" {
		t.Errorf("charge detail = %v, want the pivot-marked execute", charge.Detail)
	}
}

func TestFlatSymbolsFromSchemaIncludesMessagesSagasEnums(t *testing.T) {
	result, diags := compile.Compile(map[string]string{"main.zen": symbolsVisibilitySrc})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	symbols := flatSymbolsFromSchema(result.Schema)

	msg := findWorkspaceSymbol(symbols, "CreateUserInput")
	if msg == nil || msg.Kind != protocol.SymbolKindStruct {
		t.Fatalf("message symbol missing or wrong kind: %v", workspaceSymbolNames(symbols))
	}

	field := findWorkspaceSymbol(symbols, "email")
	if field == nil || field.Kind != protocol.SymbolKindField {
		t.Errorf("message field symbol missing or wrong kind: %v", workspaceSymbolNames(symbols))
	}

	if field.ContainerName == nil || *field.ContainerName != "CreateUserInput" {
		t.Errorf("email containerName = %v, want CreateUserInput", field.ContainerName)
	}

	step := findWorkspaceSymbol(symbols, "reserve")
	if step == nil || step.Kind != protocol.SymbolKindFunction {
		t.Fatalf("saga step symbol missing or wrong kind: %v", workspaceSymbolNames(symbols))
	}

	if step.ContainerName == nil || *step.ContainerName != "Checkout" {
		t.Errorf("reserve containerName = %v, want Checkout", step.ContainerName)
	}

	if enum := findWorkspaceSymbol(symbols, "Status"); enum == nil || enum.Kind != protocol.SymbolKindEnum {
		t.Errorf("enum symbol missing or wrong kind: %v", workspaceSymbolNames(symbols))
	}
}

func TestSymbolsFromASTIncludesMessageEnumSagaDecl(t *testing.T) {
	file, _ := parser.New("main.zen", []byte(symbolsVisibilitySrc)).ParseFile()
	if file == nil {
		t.Fatal("failed to parse fixture")
	}

	symbols := symbolsFromAST(file)

	if len(symbols) != 5 {
		t.Fatalf("got %d symbols, want 5 (enum entity message service saga): %v", len(symbols), symbols)
	}

	enum := findSymbol(symbols, "Status")
	if enum == nil {
		t.Fatal("Status symbol missing")
	}

	if enum.Kind != protocol.SymbolKindEnum {
		t.Errorf("Status kind = %d, want %d", enum.Kind, protocol.SymbolKindEnum)
	}

	// The AST fallback knows each value's position, so values are children.
	if got := childNames(enum); len(got) != 2 || got[0] != "active" || got[1] != "dormant" {
		t.Errorf("enum children = %v, want [active dormant]", got)
	}

	if enum.Children[0].Kind != protocol.SymbolKindEnumMember {
		t.Errorf("active kind = %d, want %d", enum.Children[0].Kind, protocol.SymbolKindEnumMember)
	}

	saga := findSymbol(symbols, "Checkout")
	if saga == nil {
		t.Fatal("Checkout symbol missing")
	}

	if got := childNames(saga); len(got) != 2 || got[0] != "reserve" || got[1] != "charge" {
		t.Errorf("saga children = %v, want [reserve charge]", got)
	}
}

func TestFlatSymbolsFromASTIncludesMessageEnumSagaDecl(t *testing.T) {
	file, _ := parser.New("main.zen", []byte(symbolsVisibilitySrc)).ParseFile()
	if file == nil {
		t.Fatal("failed to parse fixture")
	}

	symbols := flatSymbolsFromAST("main.zen", file)

	enum := findWorkspaceSymbol(symbols, "Status")
	if enum == nil || enum.Kind != protocol.SymbolKindEnum {
		t.Fatalf("enum symbol missing or wrong kind: %v", workspaceSymbolNames(symbols))
	}

	value := findWorkspaceSymbol(symbols, "dormant")
	if value == nil || value.Kind != protocol.SymbolKindEnumMember {
		t.Errorf("enum value symbol missing or wrong kind: %v", workspaceSymbolNames(symbols))
	}

	if value.ContainerName == nil || *value.ContainerName != "Status" {
		t.Errorf("dormant containerName = %v, want Status", value.ContainerName)
	}

	if msg := findWorkspaceSymbol(symbols, "CreateUserInput"); msg == nil || msg.Kind != protocol.SymbolKindStruct {
		t.Errorf("message symbol missing or wrong kind: %v", workspaceSymbolNames(symbols))
	}

	if step := findWorkspaceSymbol(symbols, "charge"); step == nil || step.Kind != protocol.SymbolKindFunction {
		t.Errorf("saga step symbol missing or wrong kind: %v", workspaceSymbolNames(symbols))
	}
}

// paramsSymbolsSrc exercises parameter children on both carriers: rpcs with
// a message-ref param, a scalar param and a validated param, plus a job
// with params and a parameterless job that must stay a leaf.
const paramsSymbolsSrc = `entity User {
	id: uuid @primary
}

message CreateUserInput {
	email: string @validate(format: "email")
}

service UserService {
	rpc CreateUser(input: CreateUserInput) -> User {
		http: POST "/users"
		auth: required
	}

	rpc Rename(id: uuid, name: string @validate(min_len: 1)) -> User {
		http: POST "/users/{id}/rename"
		auth: required
	}
}

job SendDigest(target: string, batch: int64) {
	queue: low
}

job Flush() {
	queue: low
}
`

// compileParamsSymbolsFixture compiles paramsSymbolsSrc and returns its
// document symbols, failing the test on diagnostics.
func compileParamsSymbolsFixture(t *testing.T) protocol.DocumentSymbolSlice {
	t.Helper()

	result, diags := compile.Compile(map[string]string{"main.zen": paramsSymbolsSrc})
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	return symbolsFromSchema(result.Schema, "main.zen")
}

// checkParamChild asserts one parameter child by name, kind and detail.
func checkParamChild(t *testing.T, parent *protocol.DocumentSymbol, name, detail string) {
	t.Helper()

	var found *protocol.DocumentSymbol

	for i := range parent.Children {
		if parent.Children[i].Name == name {
			found = &parent.Children[i]

			break
		}
	}

	if found == nil {
		t.Fatalf("%s children = %v, want a %q parameter", parent.Name, childNames(parent), name)
	}

	if found.Kind != symbolKindParameter {
		t.Errorf("%s.%s kind = %d, want parameter kind %d", parent.Name, name, found.Kind, symbolKindParameter)
	}

	if found.Detail == nil || *found.Detail != detail {
		t.Errorf("%s.%s detail = %v, want %q", parent.Name, name, found.Detail, detail)
	}
}

// TestSymbolsFromSchemaIncludesParamChildren proves resolved rpc and job
// symbols carry their parameters as children, with the referenced type name
// for a message-ref param (the `u: User` shape) and the scalar spelling
// otherwise.
func TestSymbolsFromSchemaIncludesParamChildren(t *testing.T) {
	symbols := compileParamsSymbolsFixture(t)

	svc := findSymbol(symbols, "UserService")
	if svc == nil {
		t.Fatal("UserService symbol missing")
	}

	create := findSymbol(svc.Children, "CreateUser")
	if create == nil {
		t.Fatal("CreateUser symbol missing")
	}

	checkParamChild(t, create, "input", "CreateUserInput")

	rename := findSymbol(svc.Children, "Rename")
	if rename == nil {
		t.Fatal("Rename symbol missing")
	}

	if got := childNames(rename); len(got) != 2 || got[0] != "id" || got[1] != "name" {
		t.Errorf("Rename children = %v, want [id name]", got)
	}

	checkParamChild(t, rename, "id", "uuid")
	checkParamChild(t, rename, "name", "string")

	job := findSymbol(symbols, "SendDigest")
	if job == nil {
		t.Fatal("SendDigest symbol missing")
	}

	if got := childNames(job); len(got) != 2 || got[0] != "target" || got[1] != "batch" {
		t.Errorf("SendDigest children = %v, want [target batch]", got)
	}

	checkParamChild(t, job, "target", "string")
	checkParamChild(t, job, "batch", "int64")

	// A parameterless job stays a leaf, the same shape it had before
	// parameters surfaced.
	flush := findSymbol(symbols, "Flush")
	if flush == nil {
		t.Fatal("Flush symbol missing")
	}

	if len(flush.Children) != 0 {
		t.Errorf("Flush children = %v, want none", childNames(flush))
	}
}

// TestSymbolsFromASTIncludesParamChildren proves the mid-edit fallback
// recovers the same parameter children from the parser alone.
func TestSymbolsFromASTIncludesParamChildren(t *testing.T) {
	file, _ := parser.New("main.zen", []byte(paramsSymbolsSrc)).ParseFile()
	if file == nil {
		t.Fatal("failed to parse fixture")
	}

	symbols := symbolsFromAST(file)

	svc := findSymbol(symbols, "UserService")
	if svc == nil {
		t.Fatal("UserService symbol missing")
	}

	create := findSymbol(svc.Children, "CreateUser")
	if create == nil {
		t.Fatal("CreateUser symbol missing")
	}

	checkParamChild(t, create, "input", "CreateUserInput")

	job := findSymbol(symbols, "SendDigest")
	if job == nil {
		t.Fatal("SendDigest symbol missing")
	}

	if got := childNames(job); len(got) != 2 || got[0] != "target" || got[1] != "batch" {
		t.Errorf("SendDigest children = %v, want [target batch]", got)
	}

	flush := findSymbol(symbols, "Flush")
	if flush == nil {
		t.Fatal("Flush symbol missing")
	}

	if len(flush.Children) != 0 {
		t.Errorf("Flush children = %v, want none", childNames(flush))
	}
}

// TestSymbolKindParameterIsVariable pins the LSP mapping compromise: the
// protocol defines no Parameter kind, so parameter children surface as
// Variable, the kind for a named value binding.
func TestSymbolKindParameterIsVariable(t *testing.T) {
	if symbolKindParameter != protocol.SymbolKindVariable {
		t.Errorf("symbolKindParameter = %d, want Variable %d", symbolKindParameter, protocol.SymbolKindVariable)
	}
}

func TestParamTypeName(t *testing.T) {
	tests := []struct {
		name string
		in   *ir.Param
		want string
	}{
		{"nil", nil, ""},
		{"scalar", &ir.Param{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}}, "uuid"},
		{
			"ref",
			&ir.Param{Name: "u", Ref: &ir.TypeRef{Entity: &ir.Entity{Name: "User"}}},
			"User",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := paramTypeName(tc.in); got != tc.want {
				t.Errorf("paramTypeName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAstParamTypeName(t *testing.T) {
	tests := []struct {
		name string
		in   *ast.ParamDecl
		want string
	}{
		{"nil", nil, ""},
		{"nil type", &ast.ParamDecl{Name: "id"}, ""},
		{"named", &ast.ParamDecl{Name: "u", Type: &ast.TypeExpr{Name: "User"}}, "User"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := astParamTypeName(tc.in); got != tc.want {
				t.Errorf("astParamTypeName() = %q, want %q", got, tc.want)
			}
		})
	}
}
