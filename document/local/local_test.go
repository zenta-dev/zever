package local

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/zenta-dev/zever/document"
)

func hasChrome() bool {
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "headless_shell"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}

	return false
}

func TestOpenDefaults(t *testing.T) {
	t.Parallel()

	d, err := New(document.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = d.Close() })

	drv, ok := d.(*driver)
	if !ok {
		t.Fatalf("New() type = %T, want *driver", d)
	}

	if drv.tmpDir != "/tmp" {
		t.Errorf("tmpDir = %q, want %q", drv.tmpDir, "/tmp")
	}

	if drv.tmpDirSet {
		t.Errorf("tmpDirSet = true, want false")
	}

	if drv.timeout != document.DefaultTimeout {
		t.Errorf("timeout = %v, want %v", drv.timeout, document.DefaultTimeout)
	}

	if drv.quality != int64(document.ClampQuality(0)) {
		t.Errorf("quality = %d, want %d", drv.quality, document.ClampQuality(0))
	}
}

func TestOpenInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := New(document.Options{Quality: 101})
	if !errors.Is(err, document.ErrInvalidOptions) {
		t.Fatalf("New() error = %v, want errors.Is ErrInvalidOptions", err)
	}

	_, err = New(document.Options{Timeout: -time.Second})
	if !errors.Is(err, document.ErrInvalidOptions) {
		t.Fatalf("New() error = %v, want errors.Is ErrInvalidOptions", err)
	}
}

func TestOpenWiring(t *testing.T) {
	t.Parallel()

	opts := document.Options{TmpDir: "/tmp", Timeout: 5 * time.Second, Quality: 75}

	d, err := New(opts)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = d.Close() })

	drv, ok := d.(*driver)
	if !ok {
		t.Fatalf("New() type = %T, want *driver", d)
	}

	if drv.tmpDir != "/tmp" {
		t.Errorf("tmpDir = %q, want %q", drv.tmpDir, "/tmp")
	}

	if !drv.tmpDirSet {
		t.Errorf("tmpDirSet = false, want true")
	}

	if drv.timeout != 5*time.Second {
		t.Errorf("timeout = %v, want %v", drv.timeout, 5*time.Second)
	}

	if drv.quality != 75 {
		t.Errorf("quality = %d, want %d", drv.quality, 75)
	}
}

func TestRenderUnsupportedFormat(t *testing.T) {
	t.Parallel()

	d, err := New(document.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = d.Close() })

	_, err = d.Render(context.Background(), []byte("<h1>hi</h1>"), document.OutputFormat("docx"))

	var ufe *document.UnsupportedFormatError
	if !errors.As(err, &ufe) {
		t.Fatalf("Render() error = %v, want errors.As UnsupportedFormatError", err)
	}

	if ufe.Format != document.OutputFormat("docx") {
		t.Errorf("Format = %q, want %q", ufe.Format, "docx")
	}
}

func TestRenderCanceledContext(t *testing.T) {
	t.Parallel()

	d, err := New(document.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = d.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = d.Render(ctx, []byte("<h1>hi</h1>"), document.FormatPDF)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Render() error = %v, want errors.Is context.Canceled", err)
	}
}

func TestRenderOversizedSource(t *testing.T) {
	t.Parallel()

	d, err := New(document.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = d.Close() })

	big := make([]byte, document.DefaultMaxSourceBytes+1)

	_, err = d.Render(context.Background(), big, document.FormatPDF)

	var sle *document.SizeLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("Render() error = %v, want errors.As SizeLimitError", err)
	}

	if sle.Size != len(big) {
		t.Errorf("Size = %d, want %d", sle.Size, len(big))
	}

	if sle.Limit != document.DefaultMaxSourceBytes {
		t.Errorf("Limit = %d, want %d", sle.Limit, document.DefaultMaxSourceBytes)
	}
}

func TestOpenNoSandboxEnv(t *testing.T) {
	t.Setenv("ZEVER_CHROMEDP_NO_SANDBOX", "1")

	d, err := New(document.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = d.Close() })
}

func TestCloseIdempotent(t *testing.T) {
	t.Parallel()

	d, err := New(document.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := d.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := d.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestRenderBadTmpDir(t *testing.T) {
	t.Parallel()

	d, err := New(document.Options{TmpDir: "/nonexistent-zever-local-test"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = d.Close() })

	_, err = d.Render(context.Background(), []byte("<h1>hi</h1>"), document.FormatPDF)
	if err == nil {
		t.Fatalf("Render() error = nil, want user data dir error")
	}
}

func TestRenderRunError(t *testing.T) {
	t.Parallel()

	d, err := New(document.Options{Timeout: time.Nanosecond})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = d.Close() })

	_, err = d.Render(context.Background(), []byte("<html><body><h1>hi</h1></body></html>"), document.FormatPDF)
	if err == nil {
		t.Fatalf("Render() error = nil, want render timeout error")
	}
}

func TestRenderPDF(t *testing.T) {
	if !hasChrome() {
		t.Skip("no Chrome binary found, skipping render test")
	}

	d, err := New(document.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = d.Close() })

	src := []byte("<html><body><h1>hi</h1></body></html>")

	out, err := d.Render(context.Background(), src, document.FormatPDF)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !bytes.HasPrefix(out, []byte("%PDF-")) {
		t.Errorf("PDF magic missing, got %q", out[:min(8, len(out))])
	}
}

func TestRenderPNG(t *testing.T) {
	if !hasChrome() {
		t.Skip("no Chrome binary found, skipping render test")
	}

	d, err := New(document.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = d.Close() })

	src := []byte("<html><body><h1>hi</h1></body></html>")

	out, err := d.Render(context.Background(), src, document.FormatPNG)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !bytes.HasPrefix(out, []byte("\x89PNG")) {
		t.Errorf("PNG magic missing, got %q", out[:min(8, len(out))])
	}
}

func TestRenderJPG(t *testing.T) {
	if !hasChrome() {
		t.Skip("no Chrome binary found, skipping render test")
	}

	d, err := New(document.Options{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = d.Close() })

	src := []byte("<html><body><h1>hi</h1></body></html>")

	out, err := d.Render(context.Background(), src, document.FormatJPG)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !bytes.HasPrefix(out, []byte("\xff\xd8")) {
		t.Errorf("JPG magic missing, got %q", out[:min(8, len(out))])
	}
}

func TestRenderJPGHighQuality(t *testing.T) {
	if !hasChrome() {
		t.Skip("no Chrome binary found, skipping render test")
	}

	d, err := New(document.Options{Quality: 100})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = d.Close() })

	src := []byte("<html><body><h1>hi</h1></body></html>")

	out, err := d.Render(context.Background(), src, document.FormatJPG)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !bytes.HasPrefix(out, []byte("\xff\xd8")) {
		t.Errorf("JPG magic missing, got %q", out[:min(8, len(out))])
	}
}

func TestRenderPNGWithTmpDir(t *testing.T) {
	if !hasChrome() {
		t.Skip("no Chrome binary found, skipping render test")
	}

	d, err := New(document.Options{TmpDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	t.Cleanup(func() { _ = d.Close() })

	src := []byte("<html><body><h1>hi</h1></body></html>")

	out, err := d.Render(context.Background(), src, document.FormatPNG)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}

	if !bytes.HasPrefix(out, []byte("\x89PNG")) {
		t.Errorf("PNG magic missing, got %q", out[:min(8, len(out))])
	}
}
