package password_test

import (
	"context"
	"testing"
)

type stubHasher struct{}

func (stubHasher) Hash(_ context.Context, p string) (string, error) {
	return "hash:" + p, nil
}

func (stubHasher) Verify(_ context.Context, _, p string) (bool, error) {
	return p == "secret", nil
}

func (stubHasher) NeedsRehash(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func TestStubHasherEmptyInputs(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	h := stubHasher{}

	cases := []struct {
		name string
		run  func() error
	}{
		{name: "hash empty", run: func() error { _, err := h.Hash(ctx, ""); return err }},
		{name: "verify empty", run: func() error { _, err := h.Verify(ctx, "", ""); return err }},
		{name: "needsrehash empty", run: func() error { _, err := h.NeedsRehash(ctx, ""); return err }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := tc.run(); err != nil {
				t.Fatalf("run() = %v, want nil", err)
			}
		})
	}

	ok, err := h.Verify(ctx, "", "")
	if err != nil {
		t.Fatalf("Verify() = %v, want nil", err)
	}

	if ok {
		t.Fatal("Verify() = true, want false for empty password")
	}
}
