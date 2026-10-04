package ir

import (
	"fmt"
	"testing"
)

// BenchmarkEntityFieldByName measures the linear field lookup both ways:
// hit (field present) and miss (absent), over a 16-field entity.
func BenchmarkEntityFieldByName(b *testing.B) {
	ent := &Entity{Name: "User"}
	for i := 0; i < 16; i++ {
		ent.Fields = append(ent.Fields, &Field{Name: fmt.Sprintf("f%d", i)})
	}

	b.Run("hit", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if f := ent.FieldByName("f15"); f == nil {
				b.Fatal("FieldByName(f15) = nil, want field")
			}
		}
	})

	b.Run("miss", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if f := ent.FieldByName("nope"); f != nil {
				b.Fatal("FieldByName(nope) != nil, want nil")
			}
		}
	})
}

// BenchmarkMessageFieldByName mirrors FieldByName for messages.
func BenchmarkMessageFieldByName(b *testing.B) {
	msg := &Message{Name: "CreateUserRequest"}
	for i := 0; i < 8; i++ {
		msg.Fields = append(msg.Fields, &Field{Name: fmt.Sprintf("f%d", i)})
	}

	b.ReportAllocs()
	for b.Loop() {
		if f := msg.FieldByName("f3"); f == nil {
			b.Fatal("FieldByName(f3) = nil, want field")
		}
	}
}

// BenchmarkErrorCodeHTTPStatus walks the full ErrorCode vocabulary; the
// switch backs both the proto and openapi backends' status mapping.
func BenchmarkErrorCodeHTTPStatus(b *testing.B) {
	codes := []ErrorCode{ECancelled, EUnknown, EInvalidArgument, EDeadlineExceeded, ENotFound,
		EAlreadyExists, EPermissionDenied, EResourceExhausted, EFailedPrecondition, EAborted,
		EOutOfRange, EUnimplemented, EInternal, EUnavailable, EDataLoss, EUnauthenticated}

	b.ReportAllocs()
	for b.Loop() {
		for _, c := range codes {
			_ = c.HTTPStatus()
		}
	}
}

// BenchmarkErrorCodeGRPCName is the name-mapping twin of HTTPStatus.
func BenchmarkErrorCodeGRPCName(b *testing.B) {
	codes := []ErrorCode{ECancelled, EUnknown, EInvalidArgument, EDeadlineExceeded, ENotFound,
		EAlreadyExists, EPermissionDenied, EResourceExhausted, EFailedPrecondition, EAborted,
		EOutOfRange, EUnimplemented, EInternal, EUnavailable, EDataLoss, EUnauthenticated}

	b.ReportAllocs()
	for b.Loop() {
		for _, c := range codes {
			_ = c.GRPCName()
		}
	}
}

// BenchmarkTypeRefAccessors measures the TypeRef union helpers every
// backend calls per operation.
func BenchmarkTypeRefAccessors(b *testing.B) {
	ent := &Entity{Name: "User"}
	ref := TypeRef{Entity: ent}

	b.ReportAllocs()
	for b.Loop() {
		_ = ref.IsEntity()
		_ = ref.IsMessage()
		_ = ref.Name()
		_ = ref.Module()
	}
}

// BenchmarkParamIsRef measures the param union check.
func BenchmarkParamIsRef(b *testing.B) {
	scalar := &Param{Name: "id"}
	refParam := &Param{Name: "user", Ref: &TypeRef{}}

	b.ReportAllocs()
	for b.Loop() {
		_ = scalar.IsRef()
		_ = refParam.IsRef()
	}
}
