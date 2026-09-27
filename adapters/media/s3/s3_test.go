package s3

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/gif"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	s3sdk "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"github.com/zenta-dev/zever/core/media"
)

const testID = "0123456789abcdef0123456789abcdef"

type stubTransport struct {
	mu       sync.Mutex
	calls    int
	gets     int
	puts     int
	do       func(*http.Request) (int, string, http.Header, error)
	failBody bool
}

func (s *stubTransport) Do(r *http.Request) (*http.Response, error) {
	s.mu.Lock()
	s.calls++
	if r.Method == http.MethodGet && r.URL.Query().Get("list-type") != "2" {
		s.gets++
	}
	if r.Method == http.MethodPut {
		s.puts++
	}
	failBody := s.failBody
	s.mu.Unlock()
	if failBody && r.Method == http.MethodGet && r.URL.Query().Get("list-type") != "2" {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       errReader{},
			Request:    r,
		}, nil
	}
	status, body, hdr, err := s.do(r)
	if err != nil {
		return nil, err
	}
	if hdr == nil {
		hdr = http.Header{}
	}
	return &http.Response{
		StatusCode:    status,
		Header:        hdr,
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       r,
	}, nil
}

func (s *stubTransport) counts() (calls, gets, puts int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.gets, s.puts
}

func listHit(key string) string {
	return `<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>b</Name>` +
		`<Prefix>` + testID + `</Prefix><KeyCount>1</KeyCount><MaxKeys>1</MaxKeys>` +
		`<IsTruncated>false</IsTruncated><Contents><Key>` + key + `</Key>` +
		`<LastModified>2024-01-01T00:00:00.000Z</LastModified><ETag>"e"</ETag><Size>4</Size>` +
		`</Contents></ListBucketResult>`
}

func listEmpty() string {
	return `<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Name>b</Name>` +
		`<Prefix>x</Prefix><KeyCount>0</KeyCount><MaxKeys>1</MaxKeys>` +
		`<IsTruncated>false</IsTruncated></ListBucketResult>`
}

func errXML(code string) string {
	return `<Error><Code>` + code + `</Code><Message>boom</Message></Error>`
}

func trouter(handlers map[string]func(*http.Request) (int, string, http.Header, error)) func(*http.Request) (int, string, http.Header, error) {
	return func(r *http.Request) (int, string, http.Header, error) {
		var op string
		switch {
		case r.URL.Query().Get("list-type") == "2":
			op = "list"
		case r.Method == http.MethodPut:
			op = "put"
		case r.Method == http.MethodHead:
			op = "head"
		case r.Method == http.MethodDelete:
			op = "delete"
		case r.Method == http.MethodGet:
			op = "get"
		default:
			op = r.Method
		}
		h := handlers[op]
		if h == nil {
			return 0, "", nil, errors.New("unexpected op " + op)
		}
		return h(r)
	}
}

func testDriver(t *testing.T, tr *stubTransport) *driver {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(t.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("ak", "sk", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	client := s3sdk.NewFromConfig(cfg, func(o *s3sdk.Options) {
		o.HTTPClient = tr
		o.Retryer = aws.NopRetryer{}
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
	})
	return &driver{
		client:      client,
		presigner:   s3sdk.NewPresignClient(client),
		bucket:      "media-test",
		maxDownload: 1 << 20,
		maxPixels:   50 << 20,
		presignTTL:  time.Hour,
		ffmpeg:      "ffmpeg",
		ffprobe:     "ffprobe",
	}
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func gifBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	var buf bytes.Buffer
	if err := gif.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func stubBin(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "fakebin")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// readBody drains and closes a stubbed request body.
func readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}

// okPut drains the request and answers 200.
func okPut() func(*http.Request) (int, string, http.Header, error) {
	return func(r *http.Request) (int, string, http.Header, error) {
		_, _ = readBody(r)
		return http.StatusOK, "", nil, nil
	}
}

const ffprobeFull = `{"format":{"duration":"1.5"},"streams":[` +
	`{"codec_type":"video","codec_name":"h264","width":640,"height":480},` +
	`{"codec_type":"audio","codec_name":"aac"}]}`

func ffprobeStub(json string) string {
	return "#!/bin/sh\necho '" + json + "'\n"
}

const ffmpegCopyStub = `#!/bin/sh
in=""
prev=""
for a in "$@"; do
  if [ "$prev" = "-i" ]; then in="$a"; fi
  prev="$a"
done
cp "$in" "$prev"
`

type stubAPIError struct {
	code string
	msg  string
}

func (e stubAPIError) Error() string                 { return e.code + ": " + e.msg }
func (e stubAPIError) ErrorCode() string             { return e.code }
func (e stubAPIError) ErrorMessage() string          { return e.msg }
func (e stubAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultUnknown }

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }

func (errReader) Close() error { return nil }

func validOpts() media.Options {
	return media.Options{Bucket: "media-test", AccessKeyID: "ak", SecretAccessKey: "sk"}
}

func TestOpenMissingBucket(t *testing.T) {
	t.Parallel()
	o := validOpts()
	o.Bucket = ""
	_, err := New(o)
	if !errors.Is(err, ErrMissingBucket) {
		t.Fatalf("New() err = %v, want ErrMissingBucket", err)
	}
}

func TestOpenMissingCredentials(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		mut  func(*media.Options)
	}{
		{name: "both empty", mut: func(o *media.Options) { o.AccessKeyID, o.SecretAccessKey = "", "" }},
		{name: "secret empty", mut: func(o *media.Options) { o.SecretAccessKey = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := validOpts()
			tc.mut(&o)
			_, err := New(o)
			if !errors.Is(err, ErrMissingCredentials) {
				t.Fatalf("New() err = %v, want ErrMissingCredentials", err)
			}
		})
	}
}

func TestOpenInvalidOptions(t *testing.T) {
	t.Parallel()
	o := validOpts()
	o.Endpoint = "://bad"
	_, err := New(o)
	if !errors.Is(err, media.ErrInvalidOptions) {
		t.Fatalf("New() err = %v, want invalid options", err)
	}
}

func TestOpenClientError(t *testing.T) {
	t.Parallel()
	o := validOpts()
	o.Endpoint = "ftp://invalid.example.com"
	_, err := New(o)
	if err == nil {
		t.Fatal("New() = nil, want NewClient error")
	}
}

func TestOpenDefaults(t *testing.T) {
	t.Parallel()
	m, err := New(validOpts())
	if err != nil {
		t.Fatal(err)
	}
	d, ok := m.(*driver)
	if !ok {
		t.Fatal("New() did not return *driver")
	}
	if got := d.client.Options().Region; got != "us-east-1" {
		t.Fatalf("Region = %q, want us-east-1", got)
	}
	if d.presignTTL != media.DefaultPresignTTL {
		t.Fatalf("presignTTL = %v, want %v", d.presignTTL, media.DefaultPresignTTL)
	}
	if d.maxDownload != media.DefaultMaxDownloadBytes {
		t.Fatalf("maxDownload = %d, want %d", d.maxDownload, media.DefaultMaxDownloadBytes)
	}
	if d.maxPixels != media.DefaultMaxPixels {
		t.Fatalf("maxPixels = %d, want %d", d.maxPixels, media.DefaultMaxPixels)
	}
	if d.ffmpeg != media.DefaultFFmpeg || d.ffprobe != media.DefaultFFProbe {
		t.Fatalf("tools = %q/%q, want ffmpeg/ffprobe", d.ffmpeg, d.ffprobe)
	}
	if d.baseURL != "" {
		t.Fatalf("baseURL = %q, want empty", d.baseURL)
	}
}

func TestOpenEndpointCustom(t *testing.T) {
	t.Parallel()
	o := validOpts()
	o.Endpoint = "http://localhost:9000"
	m, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := m.(*driver)
	if !ok {
		t.Fatal("New() did not return *driver")
	}
	ep := d.client.Options().BaseEndpoint
	if ep == nil || *ep != "http://localhost:9000" {
		t.Fatalf("BaseEndpoint = %v, want http://localhost:9000", ep)
	}
}

func TestOpenBaseURLTrim(t *testing.T) {
	t.Parallel()
	o := validOpts()
	o.BaseURL = "https://cdn.example.com///"
	m, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := m.(*driver)
	if !ok {
		t.Fatal("New() did not return *driver")
	}
	if got := d.baseURL; got != "https://cdn.example.com" {
		t.Fatalf("baseURL = %q, want trimmed", got)
	}
}

func TestUploadPresignedURL(t *testing.T) {
	t.Parallel()
	var gotPath, gotCT string
	var gotBody []byte
	tr := &stubTransport{do: func(r *http.Request) (int, string, http.Header, error) {
		gotPath = r.URL.EscapedPath()
		gotCT = r.Header.Get("Content-Type")
		var err error
		gotBody, err = readBody(r)
		if err != nil {
			return 0, "", nil, err
		}
		return http.StatusOK, "", nil, nil
	}}
	d := testDriver(t, tr)
	data := []byte("hello-media")
	a, err := d.Upload(t.Context(), "clip.mp4", data, media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotPath, a.ID+".mp4") {
		t.Fatalf("PUT path = %q, want id key", gotPath)
	}
	if gotCT != "video/mp4" {
		t.Fatalf("PUT content-type = %q, want video/mp4", gotCT)
	}
	if !bytes.Equal(gotBody, data) {
		t.Fatal("PUT body mismatch")
	}
	if a.ContentType != "video/mp4" || a.Size != int64(len(data)) || a.ID == "" {
		t.Fatalf("Asset = %+v, want fields set", a)
	}
	if !strings.Contains(a.URL, "X-Amz-Expires=3600") {
		t.Fatalf("URL = %q, want presigned with ttl", a.URL)
	}
}

func TestUploadPublicBaseURL(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: func(r *http.Request) (int, string, http.Header, error) {
		_, _ = readBody(r)
		return http.StatusOK, "", nil, nil
	}}
	d := testDriver(t, tr)
	d.baseURL = "https://cdn.example.com"
	a, err := d.Upload(t.Context(), "a.png", []byte("x"), media.UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if want := "https://cdn.example.com/" + a.ID + ".png"; a.URL != want {
		t.Fatalf("URL = %q, want %q", a.URL, want)
	}
}

func TestUploadExtCases(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, path, ct, wantExt, wantCT string
	}{
		{name: "ext from path", path: "a.png", ct: "", wantExt: ".png", wantCT: "image/png"},
		{name: "ext from type", path: "noext", ct: "image/jpeg", wantExt: ".jpg", wantCT: "image/jpeg"},
		{name: "bin octet", path: "noext", ct: "", wantExt: ".bin", wantCT: "application/octet-stream"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var gotKey string
			tr := &stubTransport{do: func(r *http.Request) (int, string, http.Header, error) {
				gotKey = r.URL.EscapedPath()
				_, _ = readBody(r)
				return http.StatusOK, "", nil, nil
			}}
			d := testDriver(t, tr)
			a, err := d.Upload(t.Context(), tc.path, []byte("x"), media.UploadOptions{ContentType: tc.ct})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(gotKey, tc.wantExt) {
				t.Fatalf("PUT key = %q, want suffix %q", gotKey, tc.wantExt)
			}
			if a.ContentType != tc.wantCT {
				t.Fatalf("ContentType = %q, want %q", a.ContentType, tc.wantCT)
			}
		})
	}
}

func TestUploadPutError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: func(*http.Request) (int, string, http.Header, error) {
		return 0, "", nil, errors.New("put boom")
	}}
	d := testDriver(t, tr)
	a, err := d.Upload(t.Context(), "a.png", []byte("x"), media.UploadOptions{})
	if err == nil || !strings.Contains(err.Error(), "s3: put") {
		t.Fatalf("Upload() err = %v, want put error", err)
	}
	if a != (media.Asset{}) {
		t.Fatalf("Asset = %+v, want zero", a)
	}
}

//nolint:tparallel // serial: mutates generateID seam.
func TestUploadGenerateIDError(t *testing.T) {
	old := generateID
	generateID = func() (string, error) { return "", errors.New("entropy boom") }
	defer func() { generateID = old }()
	tr := &stubTransport{do: func(*http.Request) (int, string, http.Header, error) {
		return http.StatusOK, "", nil, nil
	}}
	d := testDriver(t, tr)
	if _, err := d.Upload(t.Context(), "a.png", []byte("x"), media.UploadOptions{}); err == nil ||
		!strings.Contains(err.Error(), "s3: generate id") {
		t.Fatalf("Upload() err = %v, want generate-id error", err)
	}
}

func TestUploadPresignError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"put": func(r *http.Request) (int, string, http.Header, error) {
			_, _ = readBody(r)
			return http.StatusOK, "", nil, nil
		},
	})}
	d := testDriver(t, tr)
	cfg, err := config.LoadDefaultConfig(t.Context(),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("ak", "sk", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	broken := s3sdk.NewFromConfig(cfg, func(o *s3sdk.Options) {
		o.Retryer = aws.NopRetryer{}
	})
	d.presigner = s3sdk.NewPresignClient(broken)
	a, err := d.Upload(t.Context(), "a.png", []byte("x"), media.UploadOptions{})
	if err == nil || !strings.Contains(err.Error(), "s3: presign") {
		t.Fatalf("Upload() err = %v, want presign error", err)
	}
	if a != (media.Asset{}) {
		t.Fatalf("Asset = %+v, want zero", a)
	}
}

func listGetRouter(t *testing.T, key string, body []byte, ct string) func(*http.Request) (int, string, http.Header, error) {
	t.Helper()
	return trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) { return http.StatusOK, listHit(key), nil, nil },
		"get": func(_ *http.Request) (int, string, http.Header, error) {
			hdr := http.Header{"Content-Type": []string{ct}}
			return http.StatusOK, string(body), hdr, nil
		},
	})
}

func TestDownloadOK(t *testing.T) {
	t.Parallel()
	body := []byte("0123456789")
	tr := &stubTransport{do: listGetRouter(t, testID+".mp4", body, "video/mp4")}
	d := testDriver(t, tr)
	got, err := d.Download(t.Context(), testID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Fatal("Download body mismatch")
	}
}

func TestDownloadCapped(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: listGetRouter(t, testID+".mp4", []byte("0123456789"), "video/mp4")}
	d := testDriver(t, tr)
	d.maxDownload = 4
	_, err := d.Download(t.Context(), testID)
	if !errors.Is(err, media.ErrTooLarge) {
		t.Fatalf("Download() err = %v, want too large", err)
	}
}

func TestDownloadReadError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{
		do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
			"list": func(_ *http.Request) (int, string, http.Header, error) {
				return http.StatusOK, listHit(testID + ".mp4"), nil, nil
			},
		}),
		failBody: true,
	}
	d := testDriver(t, tr)
	if _, err := d.Download(t.Context(), testID); err == nil {
		t.Fatal("Download() = nil, want read error")
	}
}

func TestDownloadMissing(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) { return http.StatusOK, listEmpty(), nil, nil },
	})}
	d := testDriver(t, tr)
	_, err := d.Download(t.Context(), testID)
	if !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("Download() err = %v, want not found", err)
	}
}

func TestDownloadListError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: func(*http.Request) (int, string, http.Header, error) {
		return 0, "", nil, errors.New("list boom")
	}}
	d := testDriver(t, tr)
	_, err := d.Download(t.Context(), testID)
	if err == nil || !strings.Contains(err.Error(), "s3: list") {
		t.Fatalf("Download() err = %v, want list error", err)
	}
}

func TestDownloadRaceNotFound(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".mp4"), nil, nil
		},
		"get": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusNotFound, errXML("NoSuchKey"), nil, nil
		},
	})}
	d := testDriver(t, tr)
	_, err := d.Download(t.Context(), testID)
	if !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("Download() err = %v, want not found", err)
	}
}

func headGetRouter(key string, size int64, body []byte) func(*http.Request) (int, string, http.Header, error) {
	hdr := http.Header{}
	hdr.Set("Content-Length", itoa(size))
	return trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) { return http.StatusOK, listHit(key), nil, nil },
		"head": func(_ *http.Request) (int, string, http.Header, error) { return http.StatusOK, "", hdr, nil },
		"get": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, string(body), http.Header{"Content-Length": []string{itoa(int64(len(body)))}}, nil
		},
	})
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func TestDownloadRangeHeaders(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		size            string
		offset, length  int64
		body, wantRange string
	}{
		{name: "bounded", size: "100", offset: 10, length: 10, body: "0123456789", wantRange: "bytes=10-19"},
		{name: "to end", size: "100", offset: 10, length: 0, body: "tail", wantRange: "bytes=10-"},
		{name: "clamped", size: "12", offset: 10, length: 100, body: "89", wantRange: "bytes=10-11"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var gotRange string
			tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
				"list": func(_ *http.Request) (int, string, http.Header, error) {
					return http.StatusOK, listHit(testID + ".mp4"), nil, nil
				},
				"head": func(_ *http.Request) (int, string, http.Header, error) {
					return http.StatusOK, "", http.Header{"Content-Length": []string{tc.size}}, nil
				},
				"get": func(r *http.Request) (int, string, http.Header, error) {
					gotRange = r.Header.Get("Range")
					return http.StatusOK, tc.body, nil, nil
				},
			})}
			d := testDriver(t, tr)
			got, err := d.DownloadRange(t.Context(), testID, tc.offset, tc.length)
			if err != nil {
				t.Fatal(err)
			}
			if gotRange != tc.wantRange {
				t.Fatalf("Range = %q, want %q", gotRange, tc.wantRange)
			}
			if string(got) != tc.body {
				t.Fatalf("body = %q, want %q", got, tc.body)
			}
		})
	}
}

func TestDownloadRangeNegative(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: func(*http.Request) (int, string, http.Header, error) {
		return 0, "", nil, errors.New("must not call")
	}}
	d := testDriver(t, tr)
	for _, tc := range []struct {
		name           string
		offset, length int64
	}{
		{name: "negative offset", offset: -1, length: 10},
		{name: "negative length", offset: 0, length: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := d.DownloadRange(t.Context(), testID, tc.offset, tc.length)
			if !errors.Is(err, media.ErrInvalidRange) {
				t.Fatalf("DownloadRange() err = %v, want invalid range", err)
			}
		})
	}
	if calls, _, _ := tr.counts(); calls != 0 {
		t.Fatalf("calls = %d, want 0", calls)
	}
}

func TestDownloadRangeMissing(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) { return http.StatusOK, listEmpty(), nil, nil },
	})}
	d := testDriver(t, tr)
	_, err := d.DownloadRange(t.Context(), testID, 0, 10)
	if !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("DownloadRange() err = %v, want not found", err)
	}
}

func TestDownloadRangeHeadNotFound(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".mp4"), nil, nil
		},
		"head": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusNotFound, errXML("NotFound"), nil, nil
		},
	})}
	d := testDriver(t, tr)
	_, err := d.DownloadRange(t.Context(), testID, 0, 10)
	if !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("DownloadRange() err = %v, want not found", err)
	}
}

func TestDownloadRangeHeadError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".mp4"), nil, nil
		},
		"head": func(_ *http.Request) (int, string, http.Header, error) { return 0, "", nil, errors.New("head boom") },
	})}
	d := testDriver(t, tr)
	_, err := d.DownloadRange(t.Context(), testID, 0, 10)
	if err == nil || !strings.Contains(err.Error(), "s3: head") {
		t.Fatalf("DownloadRange() err = %v, want head error", err)
	}
}

func TestDownloadRangeOffsetAtSize(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: headGetRouter(testID+".mp4", 100, []byte("x"))}
	d := testDriver(t, tr)
	_, err := d.DownloadRange(t.Context(), testID, 100, 10)
	if !errors.Is(err, media.ErrInvalidRange) {
		t.Fatalf("DownloadRange() err = %v, want invalid range", err)
	}
	if _, gets, _ := tr.counts(); gets != 0 {
		t.Fatalf("gets = %d, want 0", gets)
	}
}

func TestDownloadRangeGetErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		status int
		code   string
		want   error
	}{
		{name: "unsatisfiable", status: 416, code: "InvalidRange", want: media.ErrInvalidRange},
		{name: "race gone", status: 404, code: "NoSuchKey", want: media.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
				"list": func(_ *http.Request) (int, string, http.Header, error) {
					return http.StatusOK, listHit(testID + ".mp4"), nil, nil
				},
				"head": func(_ *http.Request) (int, string, http.Header, error) {
					return http.StatusOK, "", http.Header{"Content-Length": []string{"100"}}, nil
				},
				"get": func(_ *http.Request) (int, string, http.Header, error) {
					return tc.status, errXML(tc.code), nil, nil
				},
			})}
			d := testDriver(t, tr)
			_, err := d.DownloadRange(t.Context(), testID, 10, 10)
			if !errors.Is(err, tc.want) {
				t.Fatalf("DownloadRange() err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestDownloadRangeGetError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".mp4"), nil, nil
		},
		"head": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, "", http.Header{"Content-Length": []string{"100"}}, nil
		},
		"get": func(_ *http.Request) (int, string, http.Header, error) { return 0, "", nil, errors.New("get boom") },
	})}
	d := testDriver(t, tr)
	_, err := d.DownloadRange(t.Context(), testID, 10, 10)
	if err == nil || !strings.Contains(err.Error(), "s3: get") {
		t.Fatalf("DownloadRange() err = %v, want get error", err)
	}
}

func TestDownloadRangeCapped(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".mp4"), nil, nil
		},
		"head": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, "", http.Header{"Content-Length": []string{"100"}}, nil
		},
		"get": func(_ *http.Request) (int, string, http.Header, error) { return http.StatusOK, "0123456789", nil, nil },
	})}
	d := testDriver(t, tr)
	d.maxDownload = 4
	_, err := d.DownloadRange(t.Context(), testID, 10, 10)
	if !errors.Is(err, media.ErrTooLarge) {
		t.Fatalf("DownloadRange() err = %v, want too large", err)
	}
}

func TestDownloadRangeReadError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{
		do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
			"list": func(_ *http.Request) (int, string, http.Header, error) {
				return http.StatusOK, listHit(testID + ".mp4"), nil, nil
			},
			"head": func(_ *http.Request) (int, string, http.Header, error) {
				return http.StatusOK, "", http.Header{"Content-Length": []string{"100"}}, nil
			},
		}),
		failBody: true,
	}
	d := testDriver(t, tr)
	if _, err := d.DownloadRange(t.Context(), testID, 10, 10); err == nil {
		t.Fatal("DownloadRange() = nil, want read error")
	}
}

func TestStatOK(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".mp4"), nil, nil
		},
		"head": func(_ *http.Request) (int, string, http.Header, error) {
			hdr := http.Header{"Content-Type": []string{"video/mp4"}, "Content-Length": []string{"42"}}
			return http.StatusOK, "", hdr, nil
		},
	})}
	d := testDriver(t, tr)
	info, err := d.Stat(t.Context(), testID)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != testID || info.Size != 42 || info.ContentType != "video/mp4" {
		t.Fatalf("Info = %+v", info)
	}
}

func TestStatContentTypeFallback(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".png"), nil, nil
		},
		"head": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, "", http.Header{"Content-Length": []string{"7"}}, nil
		},
	})}
	d := testDriver(t, tr)
	info, err := d.Stat(t.Context(), testID)
	if err != nil {
		t.Fatal(err)
	}
	if info.ContentType != "image/png" {
		t.Fatalf("ContentType = %q, want image/png", info.ContentType)
	}
}

func TestStatMissing(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) { return http.StatusOK, listEmpty(), nil, nil },
	})}
	d := testDriver(t, tr)
	_, err := d.Stat(t.Context(), testID)
	if !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("Stat() err = %v, want not found", err)
	}
}

func TestStatHeadMissing(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".mp4"), nil, nil
		},
		"head": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusNotFound, errXML("NotFound"), nil, nil
		},
	})}
	d := testDriver(t, tr)
	_, err := d.Stat(t.Context(), testID)
	if !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("Stat() err = %v, want not found", err)
	}
}

func TestOpTransportErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		key  string
		op   string
		msg  string
		want string
		call func(*driver, context.Context) error
	}{
		{
			name: "stat head", key: testID + ".mp4", op: "head", msg: "head boom", want: "s3: head",
			call: func(d *driver, ctx context.Context) error { _, err := d.Stat(ctx, testID); return err },
		},
		{
			name: "probe get", key: testID + ".png", op: "get", msg: "get boom", want: "s3: get",
			call: func(d *driver, ctx context.Context) error { _, err := d.Probe(ctx, testID); return err },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
				"list": func(_ *http.Request) (int, string, http.Header, error) {
					return http.StatusOK, listHit(tc.key), nil, nil
				},
				tc.op: func(_ *http.Request) (int, string, http.Header, error) {
					return 0, "", nil, errors.New(tc.msg)
				},
			})}
			d := testDriver(t, tr)
			err := tc.call(d, t.Context())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestDeleteOK(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".mp4"), nil, nil
		},
		"delete": func(_ *http.Request) (int, string, http.Header, error) { return http.StatusNoContent, "", nil, nil },
	})}
	d := testDriver(t, tr)
	if err := d.Delete(t.Context(), testID); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteMissingNil(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) { return http.StatusOK, listEmpty(), nil, nil },
	})}
	d := testDriver(t, tr)
	if err := d.Delete(t.Context(), testID); err != nil {
		t.Fatal(err)
	}
	if calls, _, _ := tr.counts(); calls != 1 {
		t.Fatalf("calls = %d, want 1 (list only)", calls)
	}
}

func TestDeleteListError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: func(*http.Request) (int, string, http.Header, error) {
		return 0, "", nil, errors.New("list boom")
	}}
	d := testDriver(t, tr)
	if err := d.Delete(t.Context(), testID); err == nil {
		t.Fatal("Delete() = nil, want error")
	}
}

func TestDeleteError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".mp4"), nil, nil
		},
		"delete": func(_ *http.Request) (int, string, http.Header, error) { return 0, "", nil, errors.New("delete boom") },
	})}
	d := testDriver(t, tr)
	if err := d.Delete(t.Context(), testID); err == nil || !strings.Contains(err.Error(), "s3: delete") {
		t.Fatalf("Delete() err = %v, want delete error", err)
	}
}

func TestProbeImage(t *testing.T) {
	t.Parallel()
	img := pngBytes(t, 3, 2)
	tr := &stubTransport{do: listGetRouter(t, testID+".png", img, "image/png")}
	d := testDriver(t, tr)
	p, err := d.Probe(t.Context(), testID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != media.KindImage || p.Format != "png" || p.Width != 3 || p.Height != 2 {
		t.Fatalf("Probe = %+v", p)
	}
}

func TestProbeImageCorrupt(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: listGetRouter(t, testID+".png", []byte("not-an-image"), "image/png")}
	d := testDriver(t, tr)
	_, err := d.Probe(t.Context(), testID)
	if !errors.Is(err, media.ErrUnsupportedFormat) {
		t.Fatalf("Probe() err = %v, want unsupported format", err)
	}
}

func TestProbeImageTooManyPixels(t *testing.T) {
	t.Parallel()
	img := pngBytes(t, 3, 2)
	tr := &stubTransport{do: listGetRouter(t, testID+".png", img, "image/png")}
	d := testDriver(t, tr)
	d.maxPixels = 5
	_, err := d.Probe(t.Context(), testID)
	if !errors.Is(err, media.ErrTooLarge) {
		t.Fatalf("Probe() err = %v, want too large", err)
	}
}

func TestProbeAV(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: listGetRouter(t, testID+".mp4", []byte("fake-av"), "video/mp4")}
	d := testDriver(t, tr)
	d.ffprobe = stubBin(t, ffprobeStub(ffprobeFull))
	p, err := d.Probe(t.Context(), testID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != media.KindVideo || p.Format != "mp4" || p.Width != 640 || p.Height != 480 {
		t.Fatalf("Probe = %+v", p)
	}
	if p.VideoCodec != "h264" || p.AudioCodec != "aac" {
		t.Fatalf("codecs = %+v", p)
	}
	if p.Duration != 1500*time.Millisecond {
		t.Fatalf("Duration = %v, want 1.5s", p.Duration)
	}
}

func TestProbeAVEmptyDuration(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: listGetRouter(t, testID+".mp3", []byte("fake-av"), "audio/mpeg")}
	d := testDriver(t, tr)
	d.ffprobe = stubBin(t, ffprobeStub(`{}`))
	p, err := d.Probe(t.Context(), testID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Kind != media.KindAudio || p.Duration != 0 {
		t.Fatalf("Probe = %+v", p)
	}
}

func TestProbeAVBadDuration(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: listGetRouter(t, testID+".mp4", []byte("fake-av"), "video/mp4")}
	d := testDriver(t, tr)
	d.ffprobe = stubBin(t, ffprobeStub(`{"format":{"duration":"abc"}}`))
	_, err := d.Probe(t.Context(), testID)
	if !errors.Is(err, media.ErrProbeFailed) {
		t.Fatalf("Probe() err = %v, want probe failed", err)
	}
}

func TestProbeAVToolMissing(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: listGetRouter(t, testID+".mp4", []byte("fake-av"), "video/mp4")}
	d := testDriver(t, tr)
	d.ffprobe = filepath.Join(t.TempDir(), "no-such-ffprobe")
	_, err := d.Probe(t.Context(), testID)
	if !errors.Is(err, media.ErrToolMissing) {
		t.Fatalf("Probe() err = %v, want tool missing", err)
	}
}

func TestProbeAVProbeFailed(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: listGetRouter(t, testID+".mp4", []byte("fake-av"), "video/mp4")}
	d := testDriver(t, tr)
	d.ffprobe = stubBin(t, "#!/bin/sh\nexit 1\n")
	_, err := d.Probe(t.Context(), testID)
	if !errors.Is(err, media.ErrProbeFailed) {
		t.Fatalf("Probe() err = %v, want probe failed", err)
	}
}

func TestProbeAVBadJSON(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: listGetRouter(t, testID+".mp4", []byte("fake-av"), "video/mp4")}
	d := testDriver(t, tr)
	d.ffprobe = stubBin(t, "#!/bin/sh\necho 'not json'\n")
	_, err := d.Probe(t.Context(), testID)
	if !errors.Is(err, media.ErrProbeFailed) {
		t.Fatalf("Probe() err = %v, want probe failed", err)
	}
}

func TestProbeAVDurationExceeded(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: listGetRouter(t, testID+".mp4", []byte("fake-av"), "video/mp4")}
	d := testDriver(t, tr)
	d.ffprobe = stubBin(t, ffprobeStub(`{"format":{"duration":"10"}}`))
	d.maxDuration = time.Second
	_, err := d.Probe(t.Context(), testID)
	if !errors.Is(err, media.ErrDurationExceeded) {
		t.Fatalf("Probe() err = %v, want duration exceeded", err)
	}
}

func TestProbeMissing(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) { return http.StatusOK, listEmpty(), nil, nil },
	})}
	d := testDriver(t, tr)
	_, err := d.Probe(t.Context(), testID)
	if !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("Probe() err = %v, want not found", err)
	}
}

func TestProbeUnsupported(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".bin"), nil, nil
		},
	})}
	d := testDriver(t, tr)
	_, err := d.Probe(t.Context(), testID)
	if !errors.Is(err, media.ErrUnsupportedFormat) {
		t.Fatalf("Probe() err = %v, want unsupported format", err)
	}
	if _, _, gets := tr.counts(); gets != 0 {
		t.Fatalf("gets = %d, want 0 (kind checked before fetch)", gets)
	}
}

func TestProbeTempMkdirError(t *testing.T) {
	old := mkdirTemp
	mkdirTemp = func(string, string) (string, error) { return "", errors.New("temp boom") }
	defer func() { mkdirTemp = old }()
	tr := &stubTransport{do: listGetRouter(t, testID+".mp4", []byte("fake-av"), "video/mp4")}
	d := testDriver(t, tr)
	if _, err := d.Probe(t.Context(), testID); err == nil {
		t.Fatal("Probe() = nil, want temp error")
	}
}

//nolint:tparallel // serial: mutates writeFile seam.
func TestProbeTempWriteError(t *testing.T) {
	old := writeFile
	writeFile = func(string, []byte, os.FileMode) error { return errors.New("write boom") }
	defer func() { writeFile = old }()
	tr := &stubTransport{do: listGetRouter(t, testID+".mp4", []byte("fake-av"), "video/mp4")}
	d := testDriver(t, tr)
	if _, err := d.Probe(t.Context(), testID); err == nil {
		t.Fatal("Probe() = nil, want temp error")
	}
}

func transformRouter(t *testing.T, key string, body []byte, ct string, put func(r *http.Request) (int, string, http.Header, error)) func(*http.Request) (int, string, http.Header, error) {
	t.Helper()
	return trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) { return http.StatusOK, listHit(key), nil, nil },
		"get": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, string(body), http.Header{"Content-Type": []string{ct}}, nil
		},
		"put": put,
	})
}

func TestTransformImageResize(t *testing.T) {
	t.Parallel()
	var gotKey, gotCT string
	var gotBody []byte
	tr := &stubTransport{do: transformRouter(t, testID+".png", pngBytes(t, 4, 2), "image/png",
		func(r *http.Request) (int, string, http.Header, error) {
			gotKey = r.URL.EscapedPath()
			gotCT = r.Header.Get("Content-Type")
			var err error
			gotBody, err = readBody(r)
			if err != nil {
				return 0, "", nil, err
			}
			return http.StatusOK, "", nil, nil
		})}
	d := testDriver(t, tr)
	d.baseURL = "https://cdn.example.com"
	u, err := d.Transform(t.Context(), testID, media.TransformOps{Width: "2", Height: "1"})
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSuffix(strings.TrimPrefix(gotKey, "/media-test/"), ".png")
	_ = id
	if !strings.Contains(gotKey, testID+"-") || !strings.HasSuffix(gotKey, ".png") {
		t.Fatalf("PUT key = %q, want derived key", gotKey)
	}
	if gotCT != "image/png" {
		t.Fatalf("PUT content-type = %q, want image/png", gotCT)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(gotBody))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 2 || cfg.Height != 1 {
		t.Fatalf("dims = %dx%d, want 2x1", cfg.Width, cfg.Height)
	}
	if !strings.HasPrefix(u, "https://cdn.example.com/"+testID+"-") {
		t.Fatalf("URL = %q, want derived base URL", u)
	}
}

func TestTransformImageFormat(t *testing.T) {
	t.Parallel()
	var gotKey string
	var gotBody []byte
	tr := &stubTransport{do: transformRouter(t, testID+".png", pngBytes(t, 2, 2), "image/png",
		func(r *http.Request) (int, string, http.Header, error) {
			gotKey = r.URL.EscapedPath()
			var err error
			gotBody, err = readBody(r)
			if err != nil {
				return 0, "", nil, err
			}
			return http.StatusOK, "", nil, nil
		})}
	d := testDriver(t, tr)
	if _, err := d.Transform(t.Context(), testID, media.TransformOps{Format: "jpg"}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(gotKey, ".jpg") || !strings.Contains(gotKey, "-jpg") {
		t.Fatalf("PUT key = %q, want jpg derived key", gotKey)
	}
	if len(gotBody) < 2 || gotBody[0] != 0xFF || gotBody[1] != 0xD8 {
		t.Fatal("PUT body is not JPEG")
	}
}

func TestTransformImageDims(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		ops   media.TransformOps
		wantW int
		wantH int
	}{
		{name: "exact", ops: media.TransformOps{Width: "2", Height: "1"}, wantW: 2, wantH: 1},
		{name: "width only", ops: media.TransformOps{Width: "2"}, wantW: 2, wantH: 1},
		{name: "height only", ops: media.TransformOps{Height: "1"}, wantW: 2, wantH: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var gotBody []byte
			tr := &stubTransport{do: transformRouter(t, testID+".png", pngBytes(t, 4, 2), "image/png",
				func(r *http.Request) (int, string, http.Header, error) {
					var err error
					gotBody, err = readBody(r)
					if err != nil {
						return 0, "", nil, err
					}
					return http.StatusOK, "", nil, nil
				})}
			d := testDriver(t, tr)
			if _, err := d.Transform(t.Context(), testID, tc.ops); err != nil {
				t.Fatal(err)
			}
			cfg, _, err := image.DecodeConfig(bytes.NewReader(gotBody))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Width != tc.wantW || cfg.Height != tc.wantH {
				t.Fatalf("dims = %dx%d, want %dx%d", cfg.Width, cfg.Height, tc.wantW, tc.wantH)
			}
		})
	}
}

func TestTransformImageBadDims(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		ops  media.TransformOps
	}{
		{name: "bad width", ops: media.TransformOps{Width: "abc"}},
		{name: "zero width", ops: media.TransformOps{Width: "0"}},
		{name: "bad height", ops: media.TransformOps{Height: "abc"}},
		{name: "zero height", ops: media.TransformOps{Height: "-2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := &stubTransport{do: transformRouter(t, testID+".png", pngBytes(t, 2, 2), "image/png",
				okPut())}
			d := testDriver(t, tr)
			_, err := d.Transform(t.Context(), testID, tc.ops)
			if !errors.Is(err, media.ErrInvalidTransform) {
				t.Fatalf("Transform() err = %v, want invalid transform", err)
			}
		})
	}
}

func TestTransformImageBadQuality(t *testing.T) {
	t.Parallel()
	for _, q := range []int{-1, 101} {
		tr := &stubTransport{do: transformRouter(t, testID+".png", pngBytes(t, 2, 2), "image/png",
			okPut())}
		d := testDriver(t, tr)
		_, err := d.Transform(t.Context(), testID, media.TransformOps{Quality: q})
		if !errors.Is(err, media.ErrInvalidTransform) {
			t.Fatalf("Transform(q=%d) err = %v, want invalid transform", q, err)
		}
		if _, _, puts := tr.counts(); puts != 0 {
			t.Fatalf("puts = %d, want 0", puts)
		}
	}
}

func TestTransformImageAVOps(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		ops  media.TransformOps
	}{
		{name: "bitrate", ops: media.TransformOps{Bitrate: 128}},
		{name: "crf", ops: media.TransformOps{CRF: 23}},
		{name: "offset", ops: media.TransformOps{Offset: time.Second}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := &stubTransport{do: transformRouter(t, testID+".png", pngBytes(t, 2, 2), "image/png",
				okPut())}
			d := testDriver(t, tr)
			_, err := d.Transform(t.Context(), testID, tc.ops)
			if !errors.Is(err, media.ErrInvalidTransform) {
				t.Fatalf("Transform() err = %v, want invalid transform", err)
			}
		})
	}
}

func TestTransformImageCrossKind(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: transformRouter(t, testID+".png", pngBytes(t, 2, 2), "image/png",
		okPut())}
	d := testDriver(t, tr)
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Format: "mp3"})
	if !errors.Is(err, media.ErrInvalidTransform) {
		t.Fatalf("Transform() err = %v, want invalid transform", err)
	}
}

func TestTransformImageUnsupportedFormat(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: transformRouter(t, testID+".png", pngBytes(t, 2, 2), "image/png",
		okPut())}
	d := testDriver(t, tr)
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Format: "xyz"})
	if !errors.Is(err, media.ErrUnsupportedFormat) {
		t.Fatalf("Transform() err = %v, want unsupported format", err)
	}
}

func TestTransformImageCorrupt(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: transformRouter(t, testID+".png", []byte("not-an-image"), "image/png",
		okPut())}
	d := testDriver(t, tr)
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Width: "1"})
	if !errors.Is(err, media.ErrUnsupportedFormat) {
		t.Fatalf("Transform() err = %v, want unsupported format", err)
	}
}

func TestTransformImageTooManyPixels(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: transformRouter(t, testID+".png", pngBytes(t, 4, 4), "image/png",
		okPut())}
	d := testDriver(t, tr)
	d.maxPixels = 4
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Width: "2"})
	if !errors.Is(err, media.ErrTooLarge) {
		t.Fatalf("Transform() err = %v, want too large", err)
	}
}

func TestTransformImageFetchError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".png"), nil, nil
		},
		"get": func(_ *http.Request) (int, string, http.Header, error) { return 0, "", nil, errors.New("get boom") },
	})}
	d := testDriver(t, tr)
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Width: "1"})
	if err == nil || !strings.Contains(err.Error(), "s3: get") {
		t.Fatalf("Transform() err = %v, want get error", err)
	}
}

func TestTransformImageGifUnsupported(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: transformRouter(t, testID+".gif", gifBytes(t), "image/gif",
		okPut())}
	d := testDriver(t, tr)
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Width: "1"})
	if !errors.Is(err, media.ErrUnsupportedFormat) {
		t.Fatalf("Transform() err = %v, want unsupported format", err)
	}
}

func TestTransformImagePutError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".png"), nil, nil
		},
		"get": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, string(pngBytes(t, 2, 2)), nil, nil
		},
		"put": func(_ *http.Request) (int, string, http.Header, error) { return 0, "", nil, errors.New("put boom") },
	})}
	d := testDriver(t, tr)
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Width: "1"})
	if err == nil || !strings.Contains(err.Error(), "s3: put") {
		t.Fatalf("Transform() err = %v, want put error", err)
	}
}

func TestTransformIdentity(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: transformRouter(t, testID+".png", pngBytes(t, 2, 2), "image/png",
		okPut())}
	d := testDriver(t, tr)
	u, err := d.Transform(t.Context(), testID, media.TransformOps{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(u, testID+".png") || !strings.Contains(u, "X-Amz-Signature") {
		t.Fatalf("URL = %q, want presigned source URL", u)
	}
	if _, _, puts := tr.counts(); puts != 0 {
		t.Fatalf("puts = %d, want 0 (identity needs no upload)", puts)
	}
}

func TestTransformMissing(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) { return http.StatusOK, listEmpty(), nil, nil },
	})}
	d := testDriver(t, tr)
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Width: "1"})
	if !errors.Is(err, media.ErrNotFound) {
		t.Fatalf("Transform() err = %v, want not found", err)
	}
}

func TestTransformUnsupported(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".bin"), nil, nil
		},
	})}
	d := testDriver(t, tr)
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Width: "1"})
	if !errors.Is(err, media.ErrUnsupportedFormat) {
		t.Fatalf("Transform() err = %v, want unsupported format", err)
	}
}

func TestTransformAV(t *testing.T) {
	t.Parallel()
	var gotKey, gotCT string
	var gotBody []byte
	wav := []byte("fake-wav-bytes")
	tr := &stubTransport{do: transformRouter(t, testID+".wav", wav, "audio/wav",
		func(r *http.Request) (int, string, http.Header, error) {
			gotKey = r.URL.EscapedPath()
			gotCT = r.Header.Get("Content-Type")
			var err error
			gotBody, err = readBody(r)
			if err != nil {
				return 0, "", nil, err
			}
			return http.StatusOK, "", nil, nil
		})}
	d := testDriver(t, tr)
	d.ffmpeg = stubBin(t, ffmpegCopyStub)
	u, err := d.Transform(t.Context(), testID, media.TransformOps{Format: "mp3"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotKey, testID+"-") || !strings.HasSuffix(gotKey, ".mp3") {
		t.Fatalf("PUT key = %q, want derived mp3 key", gotKey)
	}
	if gotCT != "audio/mpeg" {
		t.Fatalf("PUT content-type = %q, want audio/mpeg", gotCT)
	}
	if !bytes.Equal(gotBody, wav) {
		t.Fatal("transcoded body mismatch (fake copies input)")
	}
	if !strings.Contains(u, testID+"-") {
		t.Fatalf("URL = %q, want derived URL", u)
	}
}

func TestTransformAVFullOps(t *testing.T) {
	t.Parallel()
	var gotKey string
	tr := &stubTransport{do: transformRouter(t, testID+".wav", []byte("fake-wav"), "audio/wav",
		func(r *http.Request) (int, string, http.Header, error) {
			gotKey = r.URL.EscapedPath()
			_, _ = readBody(r)
			return http.StatusOK, "", nil, nil
		})}
	d := testDriver(t, tr)
	d.ffmpeg = stubBin(t, ffmpegCopyStub)
	ops := media.TransformOps{Bitrate: 128, CRF: 23, Offset: 1500 * time.Millisecond}
	if _, err := d.Transform(t.Context(), testID, ops); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-b128", "-crf23", "-ss1.500", ".wav"} {
		if !strings.Contains(gotKey, want) {
			t.Fatalf("PUT key = %q, want %q", gotKey, want)
		}
	}
}

func TestTransformAVImageOps(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		ops  media.TransformOps
	}{
		{name: "width", ops: media.TransformOps{Width: "100"}},
		{name: "height", ops: media.TransformOps{Height: "100"}},
		{name: "quality", ops: media.TransformOps{Quality: 80}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := &stubTransport{do: transformRouter(t, testID+".wav", []byte("fake-wav"), "audio/wav",
				okPut())}
			d := testDriver(t, tr)
			_, err := d.Transform(t.Context(), testID, tc.ops)
			if !errors.Is(err, media.ErrInvalidTransform) {
				t.Fatalf("Transform() err = %v, want invalid transform", err)
			}
		})
	}
}

func TestTransformAVNegativeOps(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		ops  media.TransformOps
	}{
		{name: "bitrate", ops: media.TransformOps{Bitrate: -1}},
		{name: "crf", ops: media.TransformOps{CRF: -1}},
		{name: "offset", ops: media.TransformOps{Offset: -time.Second}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr := &stubTransport{do: transformRouter(t, testID+".wav", []byte("fake-wav"), "audio/wav",
				okPut())}
			d := testDriver(t, tr)
			_, err := d.Transform(t.Context(), testID, tc.ops)
			if !errors.Is(err, media.ErrInvalidTransform) {
				t.Fatalf("Transform() err = %v, want invalid transform", err)
			}
		})
	}
}

func TestTransformAVCrossKind(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: transformRouter(t, testID+".wav", []byte("fake-wav"), "audio/wav",
		okPut())}
	d := testDriver(t, tr)
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Format: "mp4"})
	if !errors.Is(err, media.ErrInvalidTransform) {
		t.Fatalf("Transform() err = %v, want invalid transform", err)
	}
}

func TestTransformAVUnsupported(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: transformRouter(t, testID+".wav", []byte("fake-wav"), "audio/wav",
		okPut())}
	d := testDriver(t, tr)
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Format: "xyz"})
	if !errors.Is(err, media.ErrUnsupportedFormat) {
		t.Fatalf("Transform() err = %v, want unsupported format", err)
	}
}

func TestTransformAVToolMissing(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: transformRouter(t, testID+".wav", []byte("fake-wav"), "audio/wav",
		okPut())}
	d := testDriver(t, tr)
	d.ffmpeg = filepath.Join(t.TempDir(), "no-such-ffmpeg")
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Bitrate: 64})
	if !errors.Is(err, media.ErrToolMissing) {
		t.Fatalf("Transform() err = %v, want tool missing", err)
	}
}

func TestTransformAVTranscodeFailed(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: transformRouter(t, testID+".wav", []byte("fake-wav"), "audio/wav",
		okPut())}
	d := testDriver(t, tr)
	d.ffmpeg = stubBin(t, "#!/bin/sh\nexit 1\n")
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Bitrate: 64})
	if !errors.Is(err, media.ErrTranscodeFailed) {
		t.Fatalf("Transform() err = %v, want transcode failed", err)
	}
}

func TestTransformAVNoOutput(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: transformRouter(t, testID+".wav", []byte("fake-wav"), "audio/wav",
		okPut())}
	d := testDriver(t, tr)
	d.ffmpeg = stubBin(t, "#!/bin/sh\nexit 0\n")
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Bitrate: 64})
	if err == nil {
		t.Fatal("Transform() = nil, want missing-output error")
	}
}

func TestTransformAVFetchError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".wav"), nil, nil
		},
		"get": func(_ *http.Request) (int, string, http.Header, error) { return 0, "", nil, errors.New("get boom") },
	})}
	d := testDriver(t, tr)
	d.ffmpeg = stubBin(t, ffmpegCopyStub)
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Bitrate: 64})
	if err == nil || !strings.Contains(err.Error(), "s3: get") {
		t.Fatalf("Transform() err = %v, want get error", err)
	}
}

func TestTransformAVPutError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: trouter(map[string]func(*http.Request) (int, string, http.Header, error){
		"list": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, listHit(testID + ".wav"), nil, nil
		},
		"get": func(_ *http.Request) (int, string, http.Header, error) {
			return http.StatusOK, "fake-wav", nil, nil
		},
		"put": func(_ *http.Request) (int, string, http.Header, error) { return 0, "", nil, errors.New("put boom") },
	})}
	d := testDriver(t, tr)
	d.ffmpeg = stubBin(t, ffmpegCopyStub)
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Bitrate: 64})
	if err == nil || !strings.Contains(err.Error(), "s3: put") {
		t.Fatalf("Transform() err = %v, want put error", err)
	}
}

func TestTransformAVDurationExceeded(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: transformRouter(t, testID+".wav", []byte("fake-wav"), "audio/wav",
		okPut())}
	d := testDriver(t, tr)
	d.ffmpeg = stubBin(t, ffmpegCopyStub)
	d.ffprobe = stubBin(t, ffprobeStub(`{"format":{"duration":"10"}}`))
	d.maxDuration = time.Second
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Bitrate: 64})
	if !errors.Is(err, media.ErrDurationExceeded) {
		t.Fatalf("Transform() err = %v, want duration exceeded", err)
	}
}

func TestTransformAVGateProbeError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: transformRouter(t, testID+".wav", []byte("fake-wav"), "audio/wav",
		okPut())}
	d := testDriver(t, tr)
	d.ffmpeg = stubBin(t, ffmpegCopyStub)
	d.ffprobe = stubBin(t, "#!/bin/sh\nexit 1\n")
	d.maxDuration = time.Second
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Bitrate: 64})
	if !errors.Is(err, media.ErrProbeFailed) {
		t.Fatalf("Transform() err = %v, want probe failed", err)
	}
}

func TestTransformAVGateBadDuration(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: transformRouter(t, testID+".wav", []byte("fake-wav"), "audio/wav",
		okPut())}
	d := testDriver(t, tr)
	d.ffmpeg = stubBin(t, ffmpegCopyStub)
	d.ffprobe = stubBin(t, ffprobeStub(`{"format":{"duration":"abc"}}`))
	d.maxDuration = time.Second
	_, err := d.Transform(t.Context(), testID, media.TransformOps{Bitrate: 64})
	if !errors.Is(err, media.ErrProbeFailed) {
		t.Fatalf("Transform() err = %v, want probe failed", err)
	}
}

//nolint:tparallel // serial: mutates mkdirTemp seam.
func TestTransformTempError(t *testing.T) {
	old := mkdirTemp
	mkdirTemp = func(string, string) (string, error) { return "", errors.New("temp boom") }
	defer func() { mkdirTemp = old }()
	tr := &stubTransport{do: transformRouter(t, testID+".wav", []byte("fake-wav"), "audio/wav",
		okPut())}
	d := testDriver(t, tr)
	d.ffmpeg = stubBin(t, ffmpegCopyStub)
	if _, err := d.Transform(t.Context(), testID, media.TransformOps{Bitrate: 64}); err == nil {
		t.Fatal("Transform() = nil, want temp error")
	}
}

func TestAssetURLPresignError(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: func(*http.Request) (int, string, http.Header, error) {
		return 0, "", nil, errors.New("must not call")
	}}
	d := testDriver(t, tr)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.assetURL(ctx, "k"); err == nil {
		t.Fatal("assetURL() = nil, want presign error")
	}
	if calls, _, _ := tr.counts(); calls != 0 {
		t.Fatalf("calls = %d, want 0", calls)
	}
}

func TestIsNoSuchKey(t *testing.T) {
	t.Parallel()
	if isNoSuchKey(errors.New("boom")) {
		t.Fatal("isNoSuchKey(plain) = true, want false")
	}
	if isNoSuchKey(stubAPIError{code: "AccessDenied"}) {
		t.Fatal("isNoSuchKey(other) = true, want false")
	}
	if !isNoSuchKey(stubAPIError{code: "NoSuchKey"}) {
		t.Fatal("isNoSuchKey(NoSuchKey) = false, want true")
	}
	if !isNoSuchKey(stubAPIError{code: "NotFound"}) {
		t.Fatal("isNoSuchKey(NotFound) = false, want true")
	}
}

func TestClose(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: func(*http.Request) (int, string, http.Header, error) {
		return 0, "", nil, errors.New("must not call")
	}}
	if err := testDriver(t, tr).Close(); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrent(t *testing.T) {
	t.Parallel()
	tr := &stubTransport{do: listGetRouter(t, testID+".mp4", []byte("payload"), "video/mp4")}
	d := testDriver(t, tr)
	errs := make(chan error, 10)
	for range 10 {
		go func() { _, err := d.Download(t.Context(), testID); errs <- err }()
	}
	for range 10 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}
