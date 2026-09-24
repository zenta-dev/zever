package local

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/media"
)

func openTest(t *testing.T, opts media.Options) media.Media {
	t.Helper()

	if opts.Root == "" {
		opts.Root = t.TempDir()
	}

	m, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = m.Close() })

	return m
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()

	buf := new(bytes.Buffer)
	if err := png.Encode(buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func gifBytes(t *testing.T, w, h int) []byte {
	t.Helper()

	buf := new(bytes.Buffer)
	pal := color.Palette{color.White, color.Black}

	if err := gif.Encode(buf, image.NewPaletted(image.Rect(0, 0, w, h), pal), &gif.Options{}); err != nil {
		t.Fatal(err)
	}

	return buf.Bytes()
}

func writeChunk(buf *bytes.Buffer, typ string, data []byte) {
	var ln [4]byte

	binary.BigEndian.PutUint32(ln[:], uint32(len(data))) //nolint:gosec // test chunk payloads are tiny
	buf.Write(ln[:])
	buf.WriteString(typ)
	buf.Write(data)

	var cb [4]byte

	binary.BigEndian.PutUint32(cb[:], crc32.ChecksumIEEE(append([]byte(typ), data...)))
	buf.Write(cb[:])
}

// craftPNG builds a minimal PNG with the given IHDR dims and no IDAT.
func craftPNG(w, h uint32) []byte {
	buf := new(bytes.Buffer)
	buf.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10})

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], w)
	binary.BigEndian.PutUint32(ihdr[4:8], h)
	ihdr[8] = 8
	ihdr[9] = 2

	writeChunk(buf, "IHDR", ihdr)
	writeChunk(buf, "IEND", nil)

	return buf.Bytes()
}

// eventually polls cond until true or timeout, failing the test on expiry.
func eventually(t *testing.T, timeout, interval time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %v: %s", timeout, msg)
		}
		time.Sleep(interval)
	}
}

func destPath(t *testing.T, root, id string) string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(root, id+".*"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("no stored file for id %q", id)
	}

	return matches[0]
}

func breakAsset(t *testing.T, root, id string) {
	t.Helper()

	dest := destPath(t, root, id)

	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink("nonexistent-target", dest); err != nil {
		t.Fatal(err)
	}
}

func TestOpen_defaults(t *testing.T) {
	t.Parallel()

	m, err := New(media.Options{Root: "", BaseURL: ""})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = m.Close() })

	a, ok := m.(*adapter)
	if !ok {
		t.Fatal("not *adapter")
	}

	if a.root != media.DefaultLocalRoot {
		t.Errorf("root = %q, want %q", a.root, media.DefaultLocalRoot)
	}

	if a.baseURL != media.DefaultLocalBaseURL {
		t.Errorf("baseURL = %q, want %q", a.baseURL, media.DefaultLocalBaseURL)
	}

	if a.maxDownload != media.DefaultMaxDownloadBytes {
		t.Errorf("maxDownload = %d, want %d", a.maxDownload, media.DefaultMaxDownloadBytes)
	}

	if a.maxPixels != media.DefaultMaxPixels {
		t.Errorf("maxPixels = %d, want %d", a.maxPixels, media.DefaultMaxPixels)
	}

	if a.derivedTTL != media.DefaultDerivedTTL {
		t.Errorf("derivedTTL = %v, want %v", a.derivedTTL, media.DefaultDerivedTTL)
	}
}

func TestOpen_trimsBaseURLSlash(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{BaseURL: "/media/"})

	asset, err := m.Upload(t.Context(), "f.txt", []byte("x"), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(asset.URL, "/media/") || strings.Contains(asset.URL, "//") {
		t.Fatalf("URL %q has doubled slash", asset.URL)
	}
}

func TestOpen_invalidOptions(t *testing.T) {
	t.Parallel()

	if _, err := New(media.Options{MaxDownloadBytes: -1}); !errors.Is(err, media.ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
}

func TestOpen_badRoot(t *testing.T) {
	t.Parallel()

	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := New(media.Options{Root: f}); err == nil {
		t.Fatal("want error for file root, got nil")
	}
}

func TestOpen_blockedDerived(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "derived"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := New(media.Options{Root: root}); err == nil {
		t.Fatal("want error for blocked derived dir, got nil")
	}
}

func TestUploadDownloadRoundTrip(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{})

	asset, err := m.Upload(t.Context(), "test.txt", []byte("hello"), media.UploadOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}

	if !media.ValidHexID(asset.ID) {
		t.Fatalf("ID %q is not a hex id", asset.ID)
	}

	if asset.Size != 5 || asset.ContentType != "text/plain" {
		t.Fatalf("asset = %+v, want size 5 text/plain", asset)
	}

	got, err := m.Download(t.Context(), asset.ID)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
}

func TestUpload_extResolution(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{})
	ctx := t.Context()

	audio, err := m.Upload(ctx, "noext", []byte("data"), media.UploadOptions{ContentType: "audio/mpeg"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(audio.URL, ".mp3") {
		t.Fatalf("URL %q missing .mp3 suffix", audio.URL)
	}

	bin, err := m.Upload(ctx, "noext", []byte("data"), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(bin.URL, ".bin") {
		t.Fatalf("URL %q missing .bin suffix", bin.URL)
	}

	if bin.ContentType != "application/octet-stream" {
		t.Fatalf("ContentType = %q, want octet-stream", bin.ContentType)
	}

	ext, err := m.Upload(ctx, "a.png", []byte("data"), media.UploadOptions{ContentType: "image/jpeg"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(ext.URL, ".png") || ext.ContentType != "image/jpeg" {
		t.Fatalf("asset = %+v, want .png ext with jpeg type", ext)
	}
}

func TestUpload_uniqueness(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{})
	ctx := t.Context()
	seen := map[string]struct{}{}

	for range 50 {
		asset, err := m.Upload(ctx, "f.txt", []byte("x"), media.UploadOptions{})
		if err != nil {
			t.Fatal(err)
		}

		if _, dup := seen[asset.ID]; dup {
			t.Fatalf("duplicate ID %q", asset.ID)
		}

		seen[asset.ID] = struct{}{}
	}
}

func TestUpload_concurrent(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{})
	ctx := t.Context()

	var wg sync.WaitGroup

	errs := make(chan error, 20)

	for range 20 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			_, err := m.Upload(ctx, "f.txt", []byte("x"), media.UploadOptions{})
			errs <- err
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestUpload_seamFailure(t *testing.T) {
	root := t.TempDir()
	m := openTest(t, media.Options{Root: root})

	orig := generateID
	generateID = func() (string, error) { return "", errors.New("entropy unavailable") }

	defer func() { generateID = orig }()

	if _, err := m.Upload(t.Context(), "f.txt", []byte("x"), media.UploadOptions{}); err == nil {
		t.Fatal("want error when id generation fails, got nil")
	} else if !strings.Contains(err.Error(), "generate id") {
		t.Fatalf("err = %v, want generate id context", err)
	}
}

func TestUpload_mkdirError(t *testing.T) {
	t.Parallel()

	f := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := &adapter{root: f, baseURL: "/media"}

	if _, err := m.Upload(t.Context(), "f.txt", []byte("x"), media.UploadOptions{}); err == nil {
		t.Fatal("want mkdir error, got nil")
	}
}

func TestUpload_writeError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; write to read-only dir cannot fail")
	}

	t.Parallel()

	root := t.TempDir()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	m := &adapter{root: root, baseURL: "/media"}

	if _, err := m.Upload(t.Context(), "f.txt", []byte("x"), media.UploadOptions{}); err == nil {
		t.Fatal("want write error, got nil")
	}
}

func TestDownload_capped(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{MaxDownloadBytes: 8})

	asset, err := m.Upload(t.Context(), "big.bin", bytes.Repeat([]byte("x"), 100), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.Download(t.Context(), asset.ID)
	if !errors.Is(err, media.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}

	var sle *media.SizeLimitError
	if !errors.As(err, &sle) {
		t.Fatalf("err %T is not *SizeLimitError", err)
	}

	if sle.Size != 9 || sle.Limit != 8 {
		t.Fatalf("carried = {%d %d}, want {9 8}", sle.Size, sle.Limit)
	}
}

func TestDownload_brokenLink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	m := openTest(t, media.Options{Root: root})

	asset, err := m.Upload(t.Context(), "f.txt", []byte("x"), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	breakAsset(t, root, asset.ID)

	if _, err := m.Download(t.Context(), asset.ID); err == nil {
		t.Fatal("want open error, got nil")
	}
}

func TestDownload_dirReadError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	m := openTest(t, media.Options{Root: root})

	asset, err := m.Upload(t.Context(), "f.txt", []byte("x"), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	dest := destPath(t, root, asset.ID)
	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Download(t.Context(), asset.ID); err == nil {
		t.Fatal("want read error, got nil")
	}
}

func TestGlobError(t *testing.T) {
	t.Parallel()

	m := &adapter{root: filepath.Join(t.TempDir(), "bad[")}
	ctx := t.Context()
	id := strings.Repeat("a", 32)

	if _, err := m.Download(ctx, id); err == nil {
		t.Fatal("want glob error on download, got nil")
	}

	if err := m.Delete(ctx, id); err == nil {
		t.Fatal("want glob error on delete, got nil")
	}
}

func TestDeleteRemovesAndIdempotent(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{})

	asset, err := m.Upload(t.Context(), "test.txt", []byte("data"), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if err := m.Delete(t.Context(), asset.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Download(t.Context(), asset.ID); !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("post-delete download err = %v, want ErrNotFound", err)
	}

	if err := m.Delete(t.Context(), asset.ID); err != nil {
		t.Fatalf("second delete err = %v, want nil", err)
	}
}

func TestDelete_removeError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; removal in read-only dir cannot fail")
	}

	t.Parallel()

	root := t.TempDir()
	m := openTest(t, media.Options{Root: root})

	asset, err := m.Upload(t.Context(), "test.txt", []byte("data"), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })

	if err := m.Delete(t.Context(), asset.ID); err == nil {
		t.Fatal("want remove error, got nil")
	}
}

func TestStatRoundTrip(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{})

	asset, err := m.Upload(t.Context(), "img.png", pngBytes(t, 4, 2), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	info, err := m.Stat(t.Context(), asset.ID)
	if err != nil {
		t.Fatal(err)
	}

	if info.ID != asset.ID || info.Size != asset.Size || info.ContentType != "image/png" {
		t.Fatalf("info = %+v, want id/size match and image/png", info)
	}
}

func TestStat_brokenLink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	m := openTest(t, media.Options{Root: root})

	asset, err := m.Upload(t.Context(), "f.txt", []byte("x"), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	breakAsset(t, root, asset.ID)

	if _, err := m.Stat(t.Context(), asset.ID); err == nil {
		t.Fatal("want stat error, got nil")
	}
}

func TestRejectsBadIDs(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{})
	ctx := t.Context()

	for _, id := range []string{"", "..", "../secret.txt", "*", "abc*", strings.Repeat("z", 32), strings.Repeat("a", 31)} {
		if _, err := m.Download(ctx, id); !errors.Is(err, media.ErrInvalidID) {
			t.Errorf("download %q err = %v, want ErrInvalidID", id, err)
		}

		if err := m.Delete(ctx, id); !errors.Is(err, media.ErrInvalidID) {
			t.Errorf("delete %q err = %v, want ErrInvalidID", id, err)
		}

		if _, err := m.Stat(ctx, id); !errors.Is(err, media.ErrInvalidID) {
			t.Errorf("stat %q err = %v, want ErrInvalidID", id, err)
		}

		if _, err := m.DownloadRange(ctx, id, 0, 0); !errors.Is(err, media.ErrInvalidID) {
			t.Errorf("range %q err = %v, want ErrInvalidID", id, err)
		}

		if _, err := m.Probe(ctx, id); !errors.Is(err, media.ErrInvalidID) {
			t.Errorf("probe %q err = %v, want ErrInvalidID", id, err)
		}

		if _, err := m.Transform(ctx, id, media.TransformOps{}); !errors.Is(err, media.ErrInvalidID) {
			t.Errorf("transform %q err = %v, want ErrInvalidID", id, err)
		}
	}

	missing := strings.Repeat("a", 32)

	if _, err := m.Download(ctx, missing); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("download missing err = %v, want ErrNotFound", err)
	}

	if _, err := m.Stat(ctx, missing); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("stat missing err = %v, want ErrNotFound", err)
	}

	if _, err := m.DownloadRange(ctx, missing, 0, 0); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("range missing err = %v, want ErrNotFound", err)
	}

	if _, err := m.Probe(ctx, missing); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("probe missing err = %v, want ErrNotFound", err)
	}

	if _, err := m.Transform(ctx, missing, media.TransformOps{}); !errors.Is(err, media.ErrNotFound) {
		t.Errorf("transform missing err = %v, want ErrNotFound", err)
	}
}

func TestSentinelFileUntouched(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	secret := filepath.Join(dir, "secret.txt")

	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := openTest(t, media.Options{Root: filepath.Join(dir, "store")})
	ctx := t.Context()

	if _, err := m.Download(ctx, "../secret.txt"); err == nil {
		t.Fatal("want error for traversal id")
	}

	data, err := os.ReadFile(secret)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "secret" {
		t.Fatal("sentinel file outside root modified")
	}
}

func downloadOut(t *testing.T, m media.Media, url, ext string) []byte {
	t.Helper()

	outID := strings.TrimSuffix(filepath.Base(url), ext)

	data, err := m.Download(t.Context(), outID)
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func TestTransformImage(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	m := &adapter{root: root, baseURL: "/media"}
	ctx := t.Context()

	src := image.NewRGBA(image.Rect(0, 0, 400, 200))
	buf := new(bytes.Buffer)

	if err := png.Encode(buf, src); err != nil {
		t.Fatal(err)
	}

	asset, err := m.Upload(ctx, "img.png", buf.Bytes(), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		ops        media.TransformOps
		wantW      int
		wantH      int
		wantSuffix string
	}{
		{"identity", media.TransformOps{}, 400, 200, ".png"},
		{"width only", media.TransformOps{Width: "100"}, 100, 50, ".png"},
		{"height only", media.TransformOps{Height: "50"}, 100, 50, ".png"},
		{"tiny width", media.TransformOps{Width: "1"}, 1, 1, ".png"},
		{"explicit", media.TransformOps{Width: "80", Height: "40"}, 80, 40, ".png"},
		{"jpg", media.TransformOps{Format: "jpg", Width: "10"}, 10, 5, ".jpg"},
		{"JPEG upper", media.TransformOps{Format: "JPEG", Width: "10"}, 10, 5, ".jpeg"},
		{"webp", media.TransformOps{Format: "webp", Width: "10"}, 10, 5, ".webp"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			url, err := m.Transform(ctx, asset.ID, tc.ops)
			if err != nil {
				t.Fatal(err)
			}

			if !strings.HasSuffix(url, tc.wantSuffix) {
				t.Fatalf("url %q missing suffix %q", url, tc.wantSuffix)
			}

			if tc.wantSuffix != ".png" {
				return
			}

			got, _, err := image.Decode(bytes.NewReader(downloadOut(t, m, url, tc.wantSuffix)))
			if err != nil {
				t.Fatal(err)
			}

			if got.Bounds().Dx() != tc.wantW || got.Bounds().Dy() != tc.wantH {
				t.Fatalf("got %dx%d, want %dx%d", got.Bounds().Dx(), got.Bounds().Dy(), tc.wantW, tc.wantH)
			}
		})
	}
}

func TestTransformJPEGQuality(t *testing.T) {
	t.Parallel()

	m := &adapter{root: t.TempDir(), baseURL: "/media"}
	ctx := t.Context()

	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	seed := uint32(1)

	for i := range img.Pix {
		seed = seed*1664525 + 1013904223
		img.Pix[i] = byte(seed >> 24)
	}

	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		t.Fatal(err)
	}

	asset, err := m.Upload(ctx, "img.png", buf.Bytes(), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	lo, err := m.Transform(ctx, asset.ID, media.TransformOps{Format: "jpg", Quality: 1})
	if err != nil {
		t.Fatal(err)
	}

	hi, err := m.Transform(ctx, asset.ID, media.TransformOps{Format: "jpg", Quality: 100})
	if err != nil {
		t.Fatal(err)
	}

	loData := downloadOut(t, m, lo, ".jpg")
	hiData := downloadOut(t, m, hi, ".jpg")

	if len(loData) >= len(hiData) {
		t.Fatalf("want low quality smaller than high: %d >= %d", len(loData), len(hiData))
	}
}

func TestClampQuality(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		in   int
		want int
	}{
		{0, 80},
		{-1, 80},
		{1, 1},
		{50, 50},
		{100, 100},
		{101, 100},
		{255, 100},
	} {
		if got := clampQuality(tc.in); got != tc.want {
			t.Errorf("clampQuality(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestTransformRejectsGifOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	m := &adapter{root: root, baseURL: "/media"}
	ctx := t.Context()

	asset, err := m.Upload(ctx, "img.png", pngBytes(t, 2, 2), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = m.Transform(ctx, asset.ID, media.TransformOps{Format: "gif"}); !errors.Is(err, media.ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 1 {
		t.Fatalf("want only original in root, got %d files", len(entries))
	}
}

func TestTransformGifSource(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	m := &adapter{root: root, baseURL: "/media"}
	ctx := t.Context()

	asset, err := m.Upload(ctx, "anim.gif", gifBytes(t, 2, 2), media.UploadOptions{ContentType: "image/gif"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = m.Transform(ctx, asset.ID, media.TransformOps{Width: "20"}); err == nil {
		t.Fatal("want error for gif identity output, got nil")
	}

	url, err := m.Transform(ctx, asset.ID, media.TransformOps{Format: "png", Width: "20"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(url, ".png") {
		t.Fatalf("url %q missing .png suffix", url)
	}

	got, _, err := image.Decode(bytes.NewReader(downloadOut(t, m, url, ".png")))
	if err != nil {
		t.Fatal(err)
	}

	if got.Bounds().Dx() != 20 {
		t.Fatalf("width = %d, want 20", got.Bounds().Dx())
	}
}

func TestTransformRejectsUnknownSourceExt(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	m := &adapter{root: root, baseURL: "/media"}
	ctx := t.Context()

	asset, err := m.Upload(ctx, "img.xyz", pngBytes(t, 2, 2), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.Transform(ctx, asset.ID, media.TransformOps{Width: "10"}); !errors.Is(err, media.ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
	}
}

func TestTransformRejectsUnsafeFormat(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	root := filepath.Join(dir, "store")
	secret := filepath.Join(dir, "secret.txt")

	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := &adapter{root: root, baseURL: "/media"}
	ctx := t.Context()

	asset, err := m.Upload(ctx, "img.png", pngBytes(t, 2, 2), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range []string{"../evil", "x/y", "..", "a/../b", "jpg..png"} {
		if _, err = m.Transform(ctx, asset.ID, media.TransformOps{Format: f}); err == nil {
			t.Fatalf("want error for format %q", f)
		}
	}

	data, err := os.ReadFile(secret)
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "secret" {
		t.Fatal("sentinel file outside root modified")
	}
}

func TestTransformRejectsOversize(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{MaxDownloadBytes: 8})

	asset, err := m.Upload(t.Context(), "img.png", pngBytes(t, 2, 2), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.Transform(t.Context(), asset.ID, media.TransformOps{Width: "10"})
	if !errors.Is(err, media.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestTransformRejectsHugeDimensions(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{MaxPixels: 100})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "big.png", pngBytes(t, 20, 20), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.Transform(ctx, asset.ID, media.TransformOps{Width: "10"})
	if err == nil {
		t.Fatal("want error for huge dimensions, got nil")
	}

	if !strings.Contains(err.Error(), "max pixels") {
		t.Fatalf("err = %v, want max-pixels context", err)
	}

	small, err := m.Upload(ctx, "small.png", pngBytes(t, 5, 5), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.Transform(ctx, small.ID, media.TransformOps{Width: "5"}); err != nil {
		t.Fatalf("small image should succeed, got %v", err)
	}
}

func TestTransformHandcraftedBomb(t *testing.T) {
	t.Parallel()

	m := &adapter{root: t.TempDir(), baseURL: "/media"}
	ctx := t.Context()

	asset, err := m.Upload(ctx, "bomb.png", craftPNG(10000, 10000), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	_, err = m.Transform(ctx, asset.ID, media.TransformOps{Width: "10"})
	if err == nil {
		t.Fatal("want error for pixel bomb, got nil")
	}

	if !strings.Contains(err.Error(), "image dimensions 10000x10000 exceed max pixels") {
		t.Fatalf("err = %v, want verbatim dims message", err)
	}
}

func TestTransformCorruptAndTruncated(t *testing.T) {
	t.Parallel()

	m := &adapter{root: t.TempDir(), baseURL: "/media"}
	ctx := t.Context()

	corrupt, err := m.Upload(ctx, "bad.png", []byte("not an image"), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = m.Transform(ctx, corrupt.ID, media.TransformOps{Width: "10"}); err == nil {
		t.Fatal("want error for corrupt image, got nil")
	}

	truncated, err := m.Upload(ctx, "trunc.png", craftPNG(4, 2), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.Transform(ctx, truncated.ID, media.TransformOps{Width: "10"}); err == nil {
		t.Fatal("want error for truncated image, got nil")
	}
}

func TestTransformBadDims(t *testing.T) {
	t.Parallel()

	m := &adapter{root: t.TempDir(), baseURL: "/media"}
	ctx := t.Context()

	asset, err := m.Upload(ctx, "img.png", pngBytes(t, 2, 2), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	for _, ops := range []media.TransformOps{{Width: "abc"}, {Width: "-5"}, {Height: "-1"}} {
		if _, err := m.Transform(ctx, asset.ID, ops); !errors.Is(err, media.ErrInvalidTransform) {
			t.Errorf("ops %+v err = %v, want ErrInvalidTransform", ops, err)
		}
	}
}

func TestTransform_seamFailure(t *testing.T) {
	root := t.TempDir()
	m := &adapter{root: root, baseURL: "/media"}
	ctx := t.Context()

	asset, err := m.Upload(ctx, "img.png", pngBytes(t, 2, 2), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	orig := generateID
	generateID = func() (string, error) { return "", errors.New("entropy unavailable") }

	defer func() { generateID = orig }()

	if _, err := m.Transform(ctx, asset.ID, media.TransformOps{Width: "10"}); err == nil {
		t.Fatal("want error when id generation fails, got nil")
	}
}

func TestTransform_saveError(t *testing.T) {
	root := t.TempDir()
	m := &adapter{root: root, baseURL: "/media"}
	ctx := t.Context()

	asset, err := m.Upload(ctx, "img.png", pngBytes(t, 2, 2), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	const fixed = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	if err := os.Mkdir(filepath.Join(root, fixed+".png"), 0o755); err != nil {
		t.Fatal(err)
	}

	orig := generateID
	generateID = func() (string, error) { return fixed, nil }

	defer func() { generateID = orig }()

	if _, err := m.Transform(ctx, asset.ID, media.TransformOps{Width: "10"}); err == nil {
		t.Fatal("want save error, got nil")
	}
}

func TestPrepareOutput_derivedMkdirError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	blocker := filepath.Join(root, "file")

	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := &adapter{root: root, baseURL: "/media", derivedDir: filepath.Join(blocker, "derived")}

	asset, err := m.Upload(t.Context(), "img.png", pngBytes(t, 2, 2), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.Transform(t.Context(), asset.ID, media.TransformOps{Width: "10"}); err == nil {
		t.Fatal("want derived mkdir error, got nil")
	}
}

func TestLoadImage_brokenLink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	m := &adapter{root: root, baseURL: "/media"}
	ctx := t.Context()

	asset, err := m.Upload(ctx, "img.png", pngBytes(t, 2, 2), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	breakAsset(t, root, asset.ID)

	if _, err := m.Transform(ctx, asset.ID, media.TransformOps{Width: "10"}); err == nil {
		t.Fatal("want load error, got nil")
	}
}

func TestLoadImage_dirReadError(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	m := &adapter{root: root, baseURL: "/media"}
	ctx := t.Context()

	asset, err := m.Upload(ctx, "img.png", pngBytes(t, 2, 2), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	dest := destPath(t, root, asset.ID)
	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := m.Transform(ctx, asset.ID, media.TransformOps{Width: "10"}); err == nil {
		t.Fatal("want read error, got nil")
	}
}

func TestDownloadRange(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "f.txt", []byte("0123456789"), media.UploadOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name        string
		offset, len int64
		want        string
	}{
		{"middle", 2, 4, "2345"},
		{"to end", 4, 0, "456789"},
		{"full", 0, 0, "0123456789"},
		{"clamped", 8, 100, "89"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := m.DownloadRange(ctx, asset.ID, tc.offset, tc.len)
			if err != nil {
				t.Fatal(err)
			}

			if string(got) != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDownloadRange_directAdapter(t *testing.T) {
	t.Parallel()

	m := &adapter{root: t.TempDir(), baseURL: "/media"}

	asset, err := m.Upload(t.Context(), "f.txt", []byte("0123456789"), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	got, err := m.DownloadRange(t.Context(), asset.ID, 1, 3)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "123" {
		t.Fatalf("got %q, want %q", got, "123")
	}
}

func TestDownloadRange_invalid(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "f.txt", []byte("0123456789"), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name        string
		offset, len int64
		wantSize    int64
	}{
		{"negative offset", -1, 2, 0},
		{"negative length", 0, -1, 0},
		{"offset at size", 10, 1, 10},
		{"offset past size", 99, 1, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := m.DownloadRange(ctx, asset.ID, tc.offset, tc.len)
			if !errors.Is(err, media.ErrInvalidRange) {
				t.Fatalf("err = %v, want ErrInvalidRange", err)
			}

			var ire *media.InvalidRangeError
			if !errors.As(err, &ire) {
				t.Fatalf("err %T is not *InvalidRangeError", err)
			}

			if ire.Offset != tc.offset || ire.Length != tc.len || ire.Size != tc.wantSize {
				t.Fatalf("carried = {%d %d %d}, want {%d %d %d}", ire.Offset, ire.Length, ire.Size, tc.offset, tc.len, tc.wantSize)
			}
		})
	}
}

func TestDownloadRange_capped(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{MaxDownloadBytes: 4})

	asset, err := m.Upload(t.Context(), "f.txt", []byte("0123456789"), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.DownloadRange(t.Context(), asset.ID, 0, 0); !errors.Is(err, media.ErrTooLarge) {
		t.Fatalf("err = %v, want ErrTooLarge", err)
	}
}

func TestDownloadRange_brokenLink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	m := openTest(t, media.Options{Root: root})

	asset, err := m.Upload(t.Context(), "f.txt", []byte("x"), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	breakAsset(t, root, asset.ID)

	if _, err := m.DownloadRange(t.Context(), asset.ID, 0, 0); err == nil {
		t.Fatal("want read error, got nil")
	}
}

func TestProbeImage(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "img.png", pngBytes(t, 4, 2), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	probe, err := m.Probe(ctx, asset.ID)
	if err != nil {
		t.Fatal(err)
	}

	if probe.Kind != media.KindImage || probe.Format != "png" || probe.Width != 4 || probe.Height != 2 {
		t.Fatalf("probe = %+v, want image/png 4x2", probe)
	}

	jpg, err := m.Upload(ctx, "img.jpg", pngBytes(t, 4, 2), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	jprobe, err := m.Probe(ctx, jpg.ID)
	if err != nil {
		t.Fatal(err)
	}

	if jprobe.Format != "jpg" || jprobe.Width != 4 || jprobe.Height != 2 {
		t.Fatalf("probe = %+v, want jpg 4x2", jprobe)
	}
}

func TestProbe_errors(t *testing.T) {
	t.Parallel()

	m := openTest(t, media.Options{})
	ctx := t.Context()

	corrupt, err := m.Upload(ctx, "bad.png", []byte("not an image"), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err = m.Probe(ctx, corrupt.ID); !errors.Is(err, media.ErrProbeFailed) {
		t.Fatalf("err = %v, want ErrProbeFailed", err)
	}

	unknown, err := m.Upload(ctx, "f.xyz", []byte("data"), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.Probe(ctx, unknown.ID); !errors.Is(err, media.ErrUnsupportedFormat) {
		t.Fatalf("err = %v, want ErrUnsupportedFormat", err)
	}
}

func TestProbe_brokenLink(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	m := openTest(t, media.Options{Root: root})

	asset, err := m.Upload(t.Context(), "img.png", pngBytes(t, 2, 2), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}

	breakAsset(t, root, asset.ID)

	if _, err := m.Probe(t.Context(), asset.ID); err == nil {
		t.Fatal("want open error, got nil")
	}
}

func TestClose_nilStop(t *testing.T) {
	t.Parallel()

	m := &adapter{root: t.TempDir(), baseURL: "/media"}
	if err := m.Close(); err != nil {
		t.Fatalf("close err = %v", err)
	}
}

func TestCleanDerived(t *testing.T) {
	t.Parallel()

	derived := t.TempDir()
	a := &adapter{derivedDir: derived, derivedTTL: time.Minute}

	stale := filepath.Join(derived, "stale.bin")
	if err := os.WriteFile(stale, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chtimes(stale, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	fresh := filepath.Join(derived, "fresh.bin")
	if err := os.WriteFile(fresh, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(filepath.Join(derived, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink("nonexistent-target", filepath.Join(derived, "dangling")); err != nil {
		t.Fatal(err)
	}

	a.cleanDerived()

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale file remains: %v", err)
	}

	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh file removed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(derived, "sub")); err != nil {
		t.Fatalf("subdir removed: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(derived, "dangling")); err != nil {
		t.Fatalf("unstatable link should be skipped, not removed: %v", err)
	}

	(&adapter{}).cleanDerived()
	(&adapter{derivedDir: derived}).cleanDerived()
	(&adapter{derivedDir: filepath.Join(derived, "missing")}).cleanDerived()

	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("guard cases removed fresh file: %v", err)
	}
}

func TestCleanDerived_removeError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; removal in read-only dir cannot fail")
	}

	t.Parallel()

	derived := t.TempDir()
	stale := filepath.Join(derived, "stale.bin")

	if err := os.WriteFile(stale, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chtimes(stale, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(derived, 0o500); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(derived, 0o700) })

	(&adapter{derivedDir: derived, derivedTTL: time.Minute}).cleanDerived()

	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("file should remain after failed remove: %v", err)
	}
}

func TestRunJanitor_noTTL(t *testing.T) {
	t.Parallel()

	(&adapter{}).runJanitor()
}

func TestJanitorCleansDerived(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	m := openTest(t, media.Options{Root: root, DerivedTTL: 200 * time.Millisecond})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "img.png", pngBytes(t, 2, 2), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatal(err)
	}

	url, err := m.Transform(ctx, asset.ID, media.TransformOps{Format: "jpg", Width: "10"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(url, "/media/derived/") {
		t.Fatalf("url %q missing /media/derived/ prefix", url)
	}

	derivedDir := filepath.Join(root, "derived")
	eventually(t, 5*time.Second, 5*time.Millisecond, func() bool {
		entries, err := os.ReadDir(derivedDir)
		if err != nil {
			return false
		}
		return len(entries) == 0
	}, "derived files not cleaned up")

	if _, err := os.Stat(destPath(t, root, asset.ID)); err != nil {
		t.Fatalf("original removed by janitor: %v", err)
	}
}

const fakeFFmpeg = `#!/bin/sh
echo "$@" >> "$ARGS_LOG"
last=""
for a in "$@"; do last="$a"; done
printf 'fake-av-content' > "$last"
`

const fakeFFprobe = `#!/bin/sh
cat "$FAKE_PROBE_JSON"
`

const fakeFail = `#!/bin/sh
exit 1
`

func withFakeBin(t *testing.T, files map[string]string) {
	t.Helper()

	dir := t.TempDir()

	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil { //nolint:gosec // test helper installs executable fake binaries
			t.Fatal(err)
		}
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeProbeJSON(t *testing.T, doc string) {
	t.Helper()

	p := filepath.Join(t.TempDir(), "probe.json")
	if err := os.WriteFile(p, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FAKE_PROBE_JSON", p)
}

const shortProbe = `{"format": {"format_name": "mp3", "duration": "1.5"}, "streams": [{"codec_type": "audio", "codec_name": "mp3"}]}`
const longProbe = `{"format": {"format_name": "mp3", "duration": "3600.0"}, "streams": [{"codec_type": "audio", "codec_name": "mp3"}]}`

func TestAVTranscodeAudio(t *testing.T) {
	log := filepath.Join(t.TempDir(), "args.log")
	t.Setenv("ARGS_LOG", log)
	withFakeBin(t, map[string]string{"ffmpeg": fakeFFmpeg, "ffprobe": fakeFFprobe})
	writeProbeJSON(t, shortProbe)

	root := t.TempDir()
	m := openTest(t, media.Options{Root: root})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "song.mp3", []byte("ID3fake"), media.UploadOptions{ContentType: "audio/mpeg"})
	if err != nil {
		t.Fatal(err)
	}

	ops := media.TransformOps{Format: "ogg", Bitrate: 128}

	url, err := m.Transform(ctx, asset.ID, ops)
	if err != nil {
		t.Fatal(err)
	}

	want := "/media/derived/" + asset.ID + "-br128.ogg"
	if url != want {
		t.Fatalf("url = %q, want %q", url, want)
	}

	data, err := os.ReadFile(filepath.Join(root, "derived", asset.ID+"-br128.ogg"))
	if err != nil {
		t.Fatal(err)
	}

	if string(data) != "fake-av-content" {
		t.Fatalf("derived content = %q", data)
	}

	again, err := m.Transform(ctx, asset.ID, ops)
	if err != nil {
		t.Fatal(err)
	}

	if again != url {
		t.Fatalf("second transform url = %q, want idempotent %q", again, url)
	}
}

func TestAVTranscodeVideoArgs(t *testing.T) {
	log := filepath.Join(t.TempDir(), "args.log")
	t.Setenv("ARGS_LOG", log)
	withFakeBin(t, map[string]string{"ffmpeg": fakeFFmpeg, "ffprobe": fakeFFprobe})
	writeProbeJSON(t, shortProbe)

	m := openTest(t, media.Options{})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "clip.mp4", []byte("fake-mp4"), media.UploadOptions{ContentType: "video/mp4"})
	if err != nil {
		t.Fatal(err)
	}

	url, err := m.Transform(ctx, asset.ID, media.TransformOps{Width: "640", Height: "480", Format: "mp4", Bitrate: 256, CRF: 23})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(url, asset.ID+"-w640-h480-br256-crf23.mp4") {
		t.Fatalf("url = %q, want deterministic variant name", url)
	}

	args, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"-vf", "scale=640:480", "-b:a", "256k", "-crf", "23"} {
		if !strings.Contains(string(args), want) {
			t.Fatalf("ffmpeg args %q missing %q", args, want)
		}
	}
}

func TestAVTranscodeFormatOnly(t *testing.T) {
	t.Setenv("ARGS_LOG", filepath.Join(t.TempDir(), "args.log"))
	withFakeBin(t, map[string]string{"ffmpeg": fakeFFmpeg, "ffprobe": fakeFFprobe})
	writeProbeJSON(t, shortProbe)

	root := t.TempDir()
	m := openTest(t, media.Options{Root: root})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "sound.wav", []byte("fake-wav"), media.UploadOptions{ContentType: "audio/wav"})
	if err != nil {
		t.Fatal(err)
	}

	url, err := m.Transform(ctx, asset.ID, media.TransformOps{Format: "mp3"})
	if err != nil {
		t.Fatal(err)
	}

	if url != "/media/derived/"+asset.ID+".mp3" {
		t.Fatalf("url = %q, want bare deterministic name", url)
	}
}

func TestAVThumbnail(t *testing.T) {
	t.Setenv("ARGS_LOG", filepath.Join(t.TempDir(), "args.log"))
	withFakeBin(t, map[string]string{"ffmpeg": fakeFFmpeg, "ffprobe": fakeFFprobe})
	writeProbeJSON(t, shortProbe)

	m := openTest(t, media.Options{})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "clip.mp4", []byte("fake-mp4"), media.UploadOptions{ContentType: "video/mp4"})
	if err != nil {
		t.Fatal(err)
	}

	url, err := m.Transform(ctx, asset.ID, media.TransformOps{Format: "jpg", Offset: time.Second, Width: "320"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(url, "thumb") || !strings.HasSuffix(url, ".jpg") {
		t.Fatalf("url = %q, want thumb variant with .jpg", url)
	}
}

func TestAVInvalidCombos(t *testing.T) {
	withFakeBin(t, map[string]string{"ffmpeg": fakeFFmpeg, "ffprobe": fakeFFprobe})
	writeProbeJSON(t, shortProbe)

	m := openTest(t, media.Options{})
	ctx := t.Context()

	audio, err := m.Upload(ctx, "song.mp3", []byte("ID3fake"), media.UploadOptions{ContentType: "audio/mpeg"})
	if err != nil {
		t.Fatal(err)
	}

	video, err := m.Upload(ctx, "clip.mp4", []byte("fake-mp4"), media.UploadOptions{ContentType: "video/mp4"})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		id   string
		ops  media.TransformOps
	}{
		{"negative width", audio.ID, media.TransformOps{Width: "-5", Format: "ogg"}},
		{"bad height", audio.ID, media.TransformOps{Height: "abc", Format: "ogg"}},
		{"negative bitrate", audio.ID, media.TransformOps{Format: "ogg", Bitrate: -1}},
		{"crf too high", audio.ID, media.TransformOps{Format: "ogg", CRF: 99}},
		{"empty format", audio.ID, media.TransformOps{}},
		{"cross kind audio to video", audio.ID, media.TransformOps{Format: "mp4"}},
		{"unknown format", audio.ID, media.TransformOps{Format: "xyz"}},
		{"cross kind video to audio", video.ID, media.TransformOps{Format: "mp3"}},
		{"offset on audio", audio.ID, media.TransformOps{Format: "ogg", Offset: time.Second}},
		{"offset on video output", video.ID, media.TransformOps{Format: "webm", Offset: time.Second}},
		{"negative offset", audio.ID, media.TransformOps{Format: "ogg", Offset: -time.Second}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := m.Transform(ctx, tc.id, tc.ops); !errors.Is(err, media.ErrInvalidTransform) {
				t.Fatalf("err = %v, want ErrInvalidTransform", err)
			}
		})
	}
}

func TestAVMissingTool(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	m := openTest(t, media.Options{})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "song.mp3", []byte("ID3fake"), media.UploadOptions{ContentType: "audio/mpeg"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.Transform(ctx, asset.ID, media.TransformOps{Format: "ogg"}); !errors.Is(err, media.ErrToolMissing) {
		t.Fatalf("transform err = %v, want ErrToolMissing", err)
	}

	if _, err := m.Probe(ctx, asset.ID); !errors.Is(err, media.ErrToolMissing) {
		t.Fatalf("probe err = %v, want ErrToolMissing", err)
	}
}

func TestAVDurationGate(t *testing.T) {
	t.Setenv("ARGS_LOG", filepath.Join(t.TempDir(), "args.log"))
	withFakeBin(t, map[string]string{"ffmpeg": fakeFFmpeg, "ffprobe": fakeFFprobe})

	m := openTest(t, media.Options{MaxDuration: 2 * time.Second})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "song.mp3", []byte("ID3fake"), media.UploadOptions{ContentType: "audio/mpeg"})
	if err != nil {
		t.Fatal(err)
	}

	writeProbeJSON(t, longProbe)

	if _, err := m.Transform(ctx, asset.ID, media.TransformOps{Format: "ogg"}); !errors.Is(err, media.ErrDurationExceeded) {
		t.Fatalf("err = %v, want ErrDurationExceeded", err)
	}

	writeProbeJSON(t, shortProbe)

	if _, err := m.Transform(ctx, asset.ID, media.TransformOps{Format: "ogg"}); err != nil {
		t.Fatalf("short media should pass the gate, got %v", err)
	}
}

func TestAVTranscodeFFmpegFailure(t *testing.T) {
	withFakeBin(t, map[string]string{"ffmpeg": fakeFail, "ffprobe": fakeFFprobe})
	writeProbeJSON(t, shortProbe)

	m := openTest(t, media.Options{})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "song.mp3", []byte("ID3fake"), media.UploadOptions{ContentType: "audio/mpeg"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.Transform(ctx, asset.ID, media.TransformOps{Format: "ogg"}); !errors.Is(err, media.ErrTranscodeFailed) {
		t.Fatalf("err = %v, want ErrTranscodeFailed", err)
	}
}

func TestAVProbeFFprobeFailure(t *testing.T) {
	withFakeBin(t, map[string]string{"ffmpeg": fakeFFmpeg, "ffprobe": fakeFail})

	m := openTest(t, media.Options{MaxDuration: time.Second})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "song.mp3", []byte("ID3fake"), media.UploadOptions{ContentType: "audio/mpeg"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.Transform(ctx, asset.ID, media.TransformOps{Format: "ogg"}); !errors.Is(err, media.ErrProbeFailed) {
		t.Fatalf("err = %v, want ErrProbeFailed", err)
	}

	if _, err := m.Probe(ctx, asset.ID); !errors.Is(err, media.ErrProbeFailed) {
		t.Fatalf("probe err = %v, want ErrProbeFailed", err)
	}
}

func TestProbeAV(t *testing.T) {
	withFakeBin(t, map[string]string{"ffprobe": fakeFFprobe})

	m := openTest(t, media.Options{})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "clip.mp4", []byte("fake-mp4"), media.UploadOptions{ContentType: "video/mp4"})
	if err != nil {
		t.Fatal(err)
	}

	writeProbeJSON(t, `{"format": {"format_name": "mov,mp4,m4a,3gp,3g2,mj2", "duration": "90.5"}, "streams": [
		{"codec_type": "video", "codec_name": "h264", "width": 1920, "height": 1080},
		{"codec_type": "audio", "codec_name": "aac"},
		{"codec_type": "subtitle", "codec_name": "mov_text"}
	]}`)

	probe, err := m.Probe(ctx, asset.ID)
	if err != nil {
		t.Fatal(err)
	}

	if probe.Kind != media.KindVideo || probe.Format != "mov,mp4,m4a,3gp,3g2,mj2" || probe.Width != 1920 || probe.Height != 1080 {
		t.Fatalf("probe = %+v, want video/mov,mp4,... 1920x1080", probe)
	}

	if probe.AudioCodec != "aac" || probe.VideoCodec != "h264" {
		t.Fatalf("probe = %+v, want aac/h264", probe)
	}

	if probe.Duration != 90500*time.Millisecond {
		t.Fatalf("duration = %v, want 90.5s", probe.Duration)
	}
}

func TestProbeAVAudioOnly(t *testing.T) {
	withFakeBin(t, map[string]string{"ffprobe": fakeFFprobe})

	m := openTest(t, media.Options{})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "song.mp3", []byte("ID3fake"), media.UploadOptions{ContentType: "audio/mpeg"})
	if err != nil {
		t.Fatal(err)
	}

	writeProbeJSON(t, shortProbe)

	probe, err := m.Probe(ctx, asset.ID)
	if err != nil {
		t.Fatal(err)
	}

	if probe.Kind != media.KindAudio || probe.Format != "mp3" || probe.AudioCodec != "mp3" {
		t.Fatalf("probe = %+v, want audio/mp3", probe)
	}

	if probe.Duration != 1500*time.Millisecond {
		t.Fatalf("duration = %v, want 1.5s", probe.Duration)
	}
}

func TestProbeAVBadJSON(t *testing.T) {
	withFakeBin(t, map[string]string{"ffprobe": fakeFFprobe})

	m := openTest(t, media.Options{})
	ctx := t.Context()

	asset, err := m.Upload(ctx, "song.mp3", []byte("ID3fake"), media.UploadOptions{ContentType: "audio/mpeg"})
	if err != nil {
		t.Fatal(err)
	}

	writeProbeJSON(t, `not json`)

	if _, err = m.Probe(ctx, asset.ID); !errors.Is(err, media.ErrProbeFailed) {
		t.Fatalf("err = %v, want ErrProbeFailed", err)
	}

	writeProbeJSON(t, `{"format": {}, "streams": [{"codec_type": "audio", "codec_name": "mp3", "width": 0, "height": 0}]}`)

	zero, err := m.Probe(ctx, asset.ID)
	if err != nil {
		t.Fatal(err)
	}

	if zero.Duration != 0 {
		t.Fatalf("duration = %v, want 0 for missing duration", zero.Duration)
	}

	writeProbeJSON(t, `{"format": {"duration": "abc"}, "streams": [{"codec_type": "audio", "codec_name": "mp3"}]}`)

	if _, err := m.Probe(ctx, asset.ID); !errors.Is(err, media.ErrProbeFailed) {
		t.Fatalf("err = %v, want ErrProbeFailed for garbage duration", err)
	}

	writeProbeJSON(t, `{"format": {}, "streams": []}`)

	if _, err := m.Probe(ctx, asset.ID); !errors.Is(err, media.ErrProbeFailed) {
		t.Fatalf("err = %v, want ErrProbeFailed for streamless output", err)
	}
}

func TestE2EffmpegWAVtoMP3(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}

	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed")
	}

	m := openTest(t, media.Options{})
	ctx := t.Context()

	wav := filepath.Join(t.TempDir(), "sine.wav")

	if out, err := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-y", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-c:a", "pcm_s16le", "-ar", "8000", wav).CombinedOutput(); err != nil {
		t.Fatalf("generate fixture: %v: %s", err, out)
	}

	fixture, err := os.ReadFile(wav)
	if err != nil {
		t.Fatal(err)
	}

	asset, err := m.Upload(ctx, "sine.wav", fixture, media.UploadOptions{ContentType: "audio/wav"})
	if err != nil {
		t.Fatal(err)
	}

	wprobe, err := m.Probe(ctx, asset.ID)
	if err != nil {
		t.Fatal(err)
	}

	if wprobe.Kind != media.KindAudio || wprobe.Format != "wav" || wprobe.AudioCodec != "pcm_s16le" {
		t.Fatalf("wav probe = %+v, want audio/wav pcm_s16le", wprobe)
	}

	if wprobe.Duration < time.Second-100*time.Millisecond || wprobe.Duration > time.Second+100*time.Millisecond {
		t.Fatalf("wav duration = %v, want ~1s", wprobe.Duration)
	}

	url, err := m.Transform(ctx, asset.ID, media.TransformOps{Format: "mp3", Bitrate: 64})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(url, "-br64.mp3") {
		t.Fatalf("url = %q, want -br64.mp3 suffix", url)
	}

	a, ok := m.(*adapter)
	if !ok {
		t.Fatal("not *adapter")
	}

	mp3, err := os.ReadFile(filepath.Join(a.derivedDir, asset.ID+"-br64.mp3"))
	if err != nil {
		t.Fatal(err)
	}

	if len(mp3) == 0 {
		t.Fatal("transcoded mp3 is empty")
	}

	remux, err := m.Upload(ctx, "sine.mp3", mp3, media.UploadOptions{ContentType: "audio/mpeg"})
	if err != nil {
		t.Fatal(err)
	}

	mprobe, err := m.Probe(ctx, remux.ID)
	if err != nil {
		t.Fatal(err)
	}

	if mprobe.AudioCodec != "mp3" {
		t.Fatalf("mp3 probe = %+v, want codec mp3", mprobe)
	}
}
