package local

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/storage"
)

func TestReservedKeysRejected(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t, storage.Options{})

	for _, key := range []string{"obj.meta", "dir/obj.meta", ".tmp-123", "d/.tmp-x/y", ".mtmp-1"} {
		_, err := a.PresignUpload(t.Context(), "bkt", key, "text/plain", time.Hour)
		if !errors.Is(err, storage.ErrInvalidKey) {
			t.Errorf("PresignUpload(%q) err = %v, want ErrInvalidKey", key, err)
		}
	}
}

func TestPutHonorsMaxUploadBytes(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t, storage.Options{LocalOptions: storage.LocalOptions{MaxUploadBytes: 8}})

	pu, err := a.PresignUpload(t.Context(), "bkt", "big.bin", "application/octet-stream", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	rec := doServe(t, a, http.MethodPut, signedTarget(t, pu.URL), strings.NewReader("123456789"), nil)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize PUT = %d, want 413", rec.Code)
	}

	pu, err = a.PresignUpload(t.Context(), "bkt", "ok.bin", "application/octet-stream", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	rec = doServe(t, a, http.MethodPut, signedTarget(t, pu.URL), strings.NewReader("12345678"), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("boundary PUT = %d, want 204", rec.Code)
	}
}

func TestDefaultUploadLimitIs64MiB(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t, storage.Options{})
	if got := a.uploadLimit(); got != 64<<20 {
		t.Fatalf("uploadLimit = %d, want 64 MiB", got)
	}
}

func TestNegativeMaxUploadBytesRejected(t *testing.T) {
	t.Parallel()

	_, err := New(storage.Options{LocalOptions: storage.LocalOptions{
		Root: t.TempDir(), Secret: testSecret, MaxUploadBytes: -1,
	}})
	if err == nil {
		t.Fatal("want error for negative max_upload_bytes")
	}
}

func TestGetServesAttachmentAndAllowlistedType(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(t, storage.Options{})

	pu, err := a.PresignUpload(t.Context(), "bkt", "x.bin", "application/x-shockwave-flash", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if rec := doServe(t, a, http.MethodPut, signedTarget(t, pu.URL), strings.NewReader("data"), nil); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT = %d", rec.Code)
	}

	pd, err := a.PresignDownload(t.Context(), "bkt", "x.bin", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	rec := doServe(t, a, http.MethodGet, signedTarget(t, pd.URL), nil, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d", rec.Code)
	}

	if got := rec.Header().Get("Content-Disposition"); got != "attachment" {
		t.Errorf("Content-Disposition = %q, want attachment", got)
	}

	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want octet-stream for non-allowlisted declared type", got)
	}
}

func TestDefaultRootRefusesSymlink(t *testing.T) {
	old := defaultRoot
	t.Cleanup(func() { defaultRoot = old })

	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")

	if err := os.Mkdir(victim, 0o755); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(dir, "storage")
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	defaultRoot = link

	_, err := New(storage.Options{LocalOptions: storage.LocalOptions{Secret: testSecret}})
	if err == nil {
		t.Fatal("want error for symlinked default root")
	}

	fi, statErr := os.Stat(victim)
	if statErr != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("victim dir was modified: %v %v", fi, statErr)
	}
}
