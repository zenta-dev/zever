package memory_test

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/adapters/session/memory"
	"github.com/zenta-dev/zever/core/session"
)

func TestEdgeCreate_NegativeTTL_UsesDefault(t *testing.T) {
	t.Parallel()

	ttl := 30 * time.Minute
	st, err := memory.New(session.Options{TTL: ttl})
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	before := time.Now()
	s, err := st.Create(t.Context(), -time.Hour)
	if err != nil {
		t.Fatalf("Create(negative ttl) err = %v", err)
	}

	if s.ExpiresAt.Sub(before) < ttl-time.Minute || s.ExpiresAt.Sub(before) > ttl+time.Minute {
		t.Fatalf("Create(negative ttl) ExpiresAt = %v, want ~now+%v", s.ExpiresAt, ttl)
	}
}

func TestEdgeNestedData_DeepCopy(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := openDefault(t)

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	s.Data["nested"] = map[string]any{"inner": []any{1, 2}}
	s.Data["bytes"] = []byte("abc")
	s.Data["list"] = []string{"x"}

	if serr := st.Save(ctx, s); serr != nil {
		t.Fatalf("Save err = %v", serr)
	}

	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}

	nested, ok := got.Data["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested type = %T, want map[string]any", got.Data["nested"])
	}

	inner, ok := nested["inner"].([]any)
	if !ok {
		t.Fatalf("inner type = %T, want []any", nested["inner"])
	}

	inner[0] = 99

	rawBytes, ok := got.Data["bytes"].([]byte)
	if !ok {
		t.Fatalf("bytes type = %T, want []byte", got.Data["bytes"])
	}

	rawBytes[0] = 'z'

	list, ok := got.Data["list"].([]string)
	if !ok {
		t.Fatalf("list type = %T, want []string", got.Data["list"])
	}

	list[0] = "changed"

	again, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get again err = %v", err)
	}

	nested2, ok := again.Data["nested"].(map[string]any)
	if !ok {
		t.Fatalf("again nested type = %T, want map[string]any", again.Data["nested"])
	}

	inner2, ok := nested2["inner"].([]any)
	if !ok {
		t.Fatalf("again inner type = %T, want []any", nested2["inner"])
	}

	if v := inner2[0]; v != 1 {
		t.Errorf("nested inner = %v, want 1 (aliased)", v)
	}

	rawBytes2, ok := again.Data["bytes"].([]byte)
	if !ok {
		t.Fatalf("again bytes type = %T, want []byte", again.Data["bytes"])
	}

	if v := rawBytes2[0]; v != 'a' {
		t.Errorf("bytes[0] = %q, want 'a' (aliased)", v)
	}

	list2, ok := again.Data["list"].([]string)
	if !ok {
		t.Fatalf("again list type = %T, want []string", again.Data["list"])
	}

	if v := list2[0]; v != "x" {
		t.Errorf("list[0] = %q, want %q (aliased)", v, "x")
	}
}

func TestEdgeSave_NilData_NoError(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := openDefault(t)

	id := session.NewID()
	if err := st.Save(ctx, session.Session{ID: id}); err != nil {
		t.Fatalf("Save(nil Data) err = %v", err)
	}

	got, err := st.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get(nil Data) err = %v", err)
	}
	if got.Data != nil {
		t.Fatalf("Get(nil Data).Data = %v, want nil", got.Data)
	}
}

func TestEdgeLargeDataValue(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := openDefault(t)

	s, err := st.Create(ctx, time.Hour)
	if err != nil {
		t.Fatalf("Create err = %v", err)
	}

	blob := bytes.Repeat([]byte("q"), 1<<20)
	s.Data["blob"] = blob

	if serr := st.Save(ctx, s); serr != nil {
		t.Fatalf("Save err = %v", serr)
	}

	got, err := st.Get(ctx, s.ID)
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	gotBlob, ok := got.Data["blob"].([]byte)
	if !ok {
		t.Fatalf("blob type = %T, want []byte", got.Data["blob"])
	}

	if !bytes.Equal(gotBlob, blob) {
		t.Fatalf("blob round trip mismatch: got %d bytes, want %d", len(gotBlob), len(blob))
	}
}

func TestEdgeBoundary_ManySessions(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := openDefault(t)

	const n = 512

	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		s, err := st.Create(ctx, time.Hour)
		if err != nil {
			t.Fatalf("Create[%d] err = %v", i, err)
		}

		ids = append(ids, s.ID)
	}

	for i, id := range ids {
		if _, err := st.Get(ctx, id); err != nil {
			t.Fatalf("Get[%d] err = %v", i, err)
		}
	}
}

func TestEdgeConcurrent_CreateDelete(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st := openDefault(t)

	var wg sync.WaitGroup

	for i := 0; i < 16; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			s, err := st.Create(ctx, time.Hour)
			if err != nil {
				t.Errorf("Create err = %v", err)
				return
			}
			if derr := st.Delete(ctx, s.ID); derr != nil {
				t.Errorf("Delete err = %v", derr)
				return
			}
			if _, gerr := st.Get(ctx, s.ID); !errors.Is(gerr, session.ErrNotFound) {
				t.Errorf("Get after Delete err = %v, want ErrNotFound", gerr)
			}
		}()
	}

	wg.Wait()
}

func TestEdgeClose_ConcurrentIdempotent(t *testing.T) {
	t.Parallel()

	st, err := memory.New(session.Options{})
	if err != nil {
		t.Fatalf("New err = %v", err)
	}

	const n = 16

	var wg sync.WaitGroup

	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			errs[i] = st.Close()
		}(i)
	}

	wg.Wait()

	for i, cerr := range errs {
		if cerr != nil {
			t.Errorf("Close[%d] = %v, want nil", i, cerr)
		}
	}
}
