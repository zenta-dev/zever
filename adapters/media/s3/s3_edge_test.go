package s3

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/media"
)

func TestUpload_emptyData(t *testing.T) {
	t.Parallel()
	var gotKey string
	tr := &stubTransport{do: func(r *http.Request) (int, string, http.Header, error) {
		gotKey = r.URL.EscapedPath()
		return http.StatusOK, "", nil, nil
	}}
	d := testDriver(t, tr)

	a, err := d.Upload(t.Context(), "f.txt", nil, media.UploadOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatalf("Upload err = %v", err)
	}
	if a.Size != 0 {
		t.Errorf("Size = %d, want 0", a.Size)
	}
	if !strings.HasSuffix(gotKey, ".txt") {
		t.Errorf("PUT key = %q, want .txt suffix", gotKey)
	}
}

func TestUpload_emptyPathWithContentType(t *testing.T) {
	t.Parallel()
	var gotKey string
	tr := &stubTransport{do: func(r *http.Request) (int, string, http.Header, error) {
		gotKey = r.URL.EscapedPath()
		_, _ = readBody(r)
		return http.StatusOK, "", nil, nil
	}}
	d := testDriver(t, tr)

	a, err := d.Upload(t.Context(), "", []byte("x"), media.UploadOptions{ContentType: "image/png"})
	if err != nil {
		t.Fatalf("Upload err = %v", err)
	}
	if !strings.HasSuffix(gotKey, ".png") {
		t.Errorf("PUT key = %q, want .png suffix", gotKey)
	}
	if a.ContentType != "image/png" {
		t.Errorf("ContentType = %q, want image/png", a.ContentType)
	}
}

func TestNew_endpointNoScheme(t *testing.T) {
	t.Parallel()
	o := validOpts()
	o.Endpoint = "localhost:9000"
	if _, err := New(o); err == nil {
		t.Fatal("New = nil, want endpoint error")
	}
}

func TestNew_endpointUserInfo(t *testing.T) {
	t.Parallel()
	o := validOpts()
	o.Endpoint = "http://user:pass@s3.example.com"
	if _, err := New(o); err == nil {
		t.Fatal("New = nil, want endpoint error")
	}
}

func TestNew_negativeLimits(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		mut  func(*media.Options)
	}{
		{name: "max download", mut: func(o *media.Options) { o.MaxDownloadBytes = -1 }},
		{name: "max pixels", mut: func(o *media.Options) { o.MaxPixels = -1 }},
		{name: "presign ttl", mut: func(o *media.Options) { o.PresignTTL = -1 }},
		{name: "max duration", mut: func(o *media.Options) { o.MaxDuration = -1 }},
		{name: "derived ttl", mut: func(o *media.Options) { o.DerivedTTL = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := validOpts()
			tc.mut(&o)
			if _, err := New(o); !errors.Is(err, media.ErrInvalidOptions) {
				t.Fatalf("New err = %v, want ErrInvalidOptions", err)
			}
		})
	}
}

func TestProbe_emptyData(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: listGetRouter(t, testID+".png", nil, "image/png")}
	d := testDriver(t, tr)
	if _, err := d.Probe(t.Context(), testID); !errors.Is(err, media.ErrUnsupportedFormat) {
		t.Errorf("Probe empty data err = %v, want unsupported format", err)
	}
}

func TestProbe_gifImage(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: listGetRouter(t, testID+".gif", gifBytes(t), "image/gif")}
	d := testDriver(t, tr)
	p, err := d.Probe(t.Context(), testID)
	if err != nil {
		t.Fatalf("Probe err = %v", err)
	}
	if p.Kind != media.KindImage || p.Format != "gif" || p.Width != 2 || p.Height != 2 {
		t.Errorf("Probe = %+v, want 2x2 gif image", p)
	}
}

func TestTransform_imageQualityBoundaries(t *testing.T) {
	t.Parallel()
	for _, q := range []int{1, 100} {
		t.Run("q"+itoa(int64(q)), func(t *testing.T) {
			t.Parallel()
			img := pngBytes(t, 8, 8)
			tr := &stubTransport{do: func(r *http.Request) (int, string, http.Header, error) {
				switch {
				case r.URL.Query().Get("list-type") == "2":
					return http.StatusOK, listHit(testID + ".png"), nil, nil
				case r.Method == http.MethodPut:
					_, _ = readBody(r)
					return http.StatusOK, "", nil, nil
				default:
					return http.StatusOK, string(img), http.Header{"Content-Length": []string{itoa(int64(len(img)))}}, nil
				}
			}}
			d := testDriver(t, tr)
			url, err := d.Transform(t.Context(), testID, media.TransformOps{Width: "4", Height: "4", Quality: q})
			if err != nil {
				t.Fatalf("Transform quality %d err = %v", q, err)
			}
			if !strings.Contains(url, "q"+itoa(int64(q))) {
				t.Errorf("Transform quality %d URL = %q, want q%d variant", q, url, q)
			}
		})
	}
}

func TestTransform_widthOnlyProportional(t *testing.T) {
	t.Parallel()
	img := pngBytes(t, 100, 50)
	tr := &stubTransport{do: func(r *http.Request) (int, string, http.Header, error) {
		switch {
		case r.URL.Query().Get("list-type") == "2":
			return http.StatusOK, listHit(testID + ".png"), nil, nil
		case r.Method == http.MethodPut:
			_, _ = readBody(r)
			return http.StatusOK, "", nil, nil
		default:
			return http.StatusOK, string(img), http.Header{"Content-Length": []string{itoa(int64(len(img)))}}, nil
		}
	}}
	d := testDriver(t, tr)
	url, err := d.Transform(t.Context(), testID, media.TransformOps{Width: "10"})
	if err != nil {
		t.Fatalf("Transform err = %v", err)
	}
	if !strings.Contains(url, "10x5-") {
		t.Errorf("URL = %q, want 10x5 proportional variant", url)
	}
}
