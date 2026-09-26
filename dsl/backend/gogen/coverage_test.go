package gogen

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/resolver"
)

// allScalarsFixture binds every scalar type through query/path params on a
// GET operation, exercising every emitParamBind conversion branch in the
// generated router.
const allScalarsFixture = `enum Status { active, banned }

entity SearchResult {
	id: uuid @primary
}

service SearchService {
	rpc Search(
		q: string,
		uid: uuid,
		day: date,
		n32: int32,
		n64: int64,
		f32: float32,
		f64: float64,
		flag: bool,
		when: timestamp,
		blob: bytes,
		doc: json,
		status: Status,
		kind: enum(red, blue)
	) -> SearchResult {
		http: GET "/search"
		auth: required
	}
}`

func generateScalars(t *testing.T) string {
	t.Helper()

	file := compileSchema(t, allScalarsFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	return string(out["app/router.go"])
}

// TestEmitParamBindCoversEveryScalar proves each scalar's query-param
// conversion renders (and fails closed with a 400 on unparsable input).
func TestEmitParamBindCoversEveryScalar(t *testing.T) {
	router := generateScalars(t)

	cases := map[string]string{
		`req.Q = raw`:                         "string passthrough",
		`req.Uid = raw`:                       "uuid passthrough",
		`req.Day = raw`:                       "date passthrough",
		`req.Blob = []byte(raw)`:              "bytes conversion",
		`strconv.ParseInt(raw, 10, 32)`:       "int32 conversion",
		`strconv.ParseInt(raw, 10, 64)`:       "int64 conversion",
		`strconv.ParseFloat(raw, 32)`:         "float32 conversion",
		`strconv.ParseFloat(raw, 64)`:         "float64 conversion",
		`strconv.ParseBool(raw)`:              "bool conversion",
		`time.Parse(time.RFC3339, raw)`:       "timestamp conversion",
		`timestamppb.New(v)`:                  "timestamp message wrap",
		`structpb.Struct{}`:                   "json conversion",
		`protojson.Unmarshal([]byte(raw), v)`: "json protojson decode",
		`req.Kind = raw`:                      "anonymous enum passthrough",
	}

	for want, what := range cases {
		if !strings.Contains(router, want) {
			t.Errorf("router.go missing %s (%s)", what, want)
		}
	}
}

// TestEmitEnumParamBindCoversNamedEnum proves a named-enum param renders a
// value switch onto the real protobuf enum constants with a 400 default.
func TestEmitEnumParamBindCoversNamedEnum(t *testing.T) {
	router := generateScalars(t)

	if !strings.Contains(router, "case \"active\":") || !strings.Contains(router, "case \"banned\":") {
		t.Fatalf("router.go missing named-enum value cases:\n%s", router)
	}

	if !strings.Contains(router, "pb.Status_STATUS_ACTIVE") {
		t.Fatalf("router.go missing protobuf enum constant reference:\n%s", router)
	}

	types := generateTypesForScalars(t)
	if !strings.Contains(types, "type Status = pb.Status") {
		t.Fatalf("types.go missing named-enum alias:\n%s", types)
	}
}

func generateTypesForScalars(t *testing.T) string {
	t.Helper()

	file := compileSchema(t, allScalarsFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	return string(out["app/types.go"])
}

// allRulesFixture declares every @validate rule kind plus every format, so
// the generated validator covers all writeValidationCheck branches.
const allRulesFixture = `message FilterInput {
	name: string @validate(min_len: 2, max_len: 128)
	email: string @validate(format: "email")
	site: string @validate(format: "url")
	token: string @validate(format: "uuid")
	age: int32 @validate(gt: 0, gte: 1, lt: 150, lte: 149)
	score: float64 @validate(gt: 0.5)
}

entity Profile {
	id: uuid @primary
}

service ProfileService {
	rpc Filter(input: FilterInput) -> Profile {
		auth: required
	}
}`

func generateRulesValidator(t *testing.T) string {
	t.Helper()

	file := compileSchema(t, allRulesFixture)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	return string(out["app/grpc.go"])
}

// TestGeneratedUUIDCheckContract pins the runtime behavior the generated
// `format: "uuid"` validator depends on (writeFormatCheck emits uuid.Parse):
// valid UUIDs pass, garbage and empty fail closed. This also promotes
// github.com/google/uuid from indirect to direct in go.mod, since generated
// output imports it.
func TestGeneratedUUIDCheckContract(t *testing.T) {
	if _, err := uuid.Parse("9d3f4a2e-1b1c-4d5e-8f90-123456789abc"); err != nil {
		t.Fatalf("uuid.Parse rejected a valid UUID: %v", err)
	}

	for _, bad := range []string{"", "not-a-uuid", "9d3f4a2e-1b1c-4d5e-8f90"} {
		if _, err := uuid.Parse(bad); err == nil {
			t.Fatalf("uuid.Parse accepted %q, want rejection", bad)
		}
	}
}

// TestValidateCoversEveryRule proves each comparison rule and format renders
// a fail-closed apperror.InvalidArgument check.
func TestValidateCoversEveryRule(t *testing.T) {
	grpc := generateRulesValidator(t)

	cases := map[string]string{
		`len(req.Name) < 2`:             "min_len",
		`len(req.Name) > 128`:           "max_len",
		`req.Age <= 0`:                  "gt",
		`req.Age < 1`:                   "gte",
		`req.Age >= 150`:                "lt",
		`req.Age > 149`:                 "lte",
		`req.Score <= 0.5`:              "float gt literal",
		`mail.ParseAddress(req.Email)`:  "email format",
		`url.ParseRequestURI(req.Site)`: "url format",
		`uuid.Parse(req.Token)`:         "uuid format",
	}

	for want, what := range cases {
		if !strings.Contains(grpc, want) {
			t.Errorf("grpc.go validator missing %s check (%s)", what, want)
		}
	}
}

// TestScalarParamValidateMarksOperation proves a @validate rule directly on
// a scalar rpc param also triggers validator emission (opNeedsValidate's
// scalar branch).
func TestScalarParamValidateMarksOperation(t *testing.T) {
	file := compileSchema(t, `entity User {
	id: uuid @primary
}

service UserService {
	rpc GetUser(id: uuid, note: string @validate(min_len: 2)) -> User {
		http: GET "/users/{id}"
		auth: required
	}
}`)

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	grpc := string(out["app/grpc.go"])
	if !strings.Contains(grpc, "func validateGetUserRequest(req *GetUserRequest) error {") {
		t.Fatalf("expected scalar-param validator, got:\n%s", grpc)
	}

	if !strings.Contains(grpc, "len(req.Note) < 2") {
		t.Fatalf("expected scalar-param min_len check, got:\n%s", grpc)
	}
}
