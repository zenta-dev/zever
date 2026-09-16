package compile

import (
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/diag"
)

// zeroPos reports whether p is the zero diag.Position, i.e. no source
// location was ever recorded for it.
func zeroPos(p diag.Position) bool {
	return p == diag.Position{}
}

// TestResolvedIRSubStructuresCarryPositions proves that Param, AuthPolicy,
// PermissionCheck, ErrorCase, RetryPolicy, and FieldType all get a real,
// non-zero Pos once resolved from source that exercises each of them —
// closing the source-mapping gaps on IR sub-structures that sit underneath
// Entity/Operation/Service, which already had a Pos before this change.
func TestResolvedIRSubStructuresCarryPositions(t *testing.T) {
	src := `entity User {
	id: uuid @primary
	role: enum(admin, member)
}

service UserService {
	rpc GetUser(id: uuid) -> User {
		auth: required(roles: {admin})
		permission: check("owns", resource: User, owner_field: id)
		errors: { not_found, permission_denied("no access") }
	}
}

job SendDigest() {
	retry: max_attempts(3), backoff(exponential, base: 30s)
}
`

	result, diags := Compile(map[string]string{"a.zen": src})
	if diags.HasErrors() {
		t.Fatalf("Compile: unexpected errors: %v", diags)
	}

	user := findEntity(result.Schema, "User")
	if user == nil {
		t.Fatalf("entity User not found")
	}

	roleField := user.FieldByName("role")
	if roleField == nil {
		t.Fatalf("field User.role not found")
	}

	if zeroPos(roleField.Type.Pos) {
		t.Errorf("FieldType.Pos is zero for User.role's enum type, want a real position")
	}

	op := result.Schema.Modules[0].Services[0].Operations[0]

	if len(op.Params) == 0 {
		t.Fatalf("expected GetUser to have params")
	}

	if zeroPos(op.Params[0].Pos) {
		t.Errorf("Param.Pos is zero for GetUser's id param, want a real position")
	}

	if op.Auth == nil {
		t.Fatalf("expected GetUser to have an Auth policy")
	}

	if zeroPos(op.Auth.Pos) {
		t.Errorf("AuthPolicy.Pos is zero, want a real position")
	}

	if op.Permission == nil {
		t.Fatalf("expected GetUser to have a Permission check")
	}

	if zeroPos(op.Permission.Pos) {
		t.Errorf("PermissionCheck.Pos is zero, want a real position")
	}

	if len(op.Errors) != 2 {
		t.Fatalf("expected GetUser to declare 2 errors, got %d", len(op.Errors))
	}

	for i, e := range op.Errors {
		if zeroPos(e.Pos) {
			t.Errorf("ErrorCase[%d].Pos is zero, want a real position", i)
		}
	}

	job := result.Schema.Modules[0].Jobs[0]

	if zeroPos(job.Retry.Pos) {
		t.Errorf("RetryPolicy.Pos is zero, want a real position")
	}
}
