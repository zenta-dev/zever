package s3

import (
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	s3sdk "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/zenta-dev/zever/core/media"
)

func benchDriver(b *testing.B, tr *stubTransport) *driver {
	b.Helper()
	cfg, err := config.LoadDefaultConfig(b.Context(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("ak", "sk", "")),
	)
	if err != nil {
		b.Fatalf("LoadDefaultConfig err = %v", err)
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

func benchUploadRouter() func(*http.Request) (int, string, http.Header, error) {
	return func(r *http.Request) (int, string, http.Header, error) {
		_, _ = readBody(r)
		return http.StatusOK, "", nil, nil
	}
}

func BenchmarkNew(b *testing.B) {
	opts := validOpts()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := New(opts); err != nil {
			b.Fatalf("New err = %v", err)
		}
	}
}

func BenchmarkUpload(b *testing.B) {
	d := benchDriver(b, &stubTransport{do: benchUploadRouter()})
	data := []byte("bench payload")
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := d.Upload(ctx, "f.txt", data, media.UploadOptions{ContentType: "text/plain"}); err != nil {
			b.Fatalf("Upload err = %v", err)
		}
	}
}

func BenchmarkDownload(b *testing.B) {
	body := []byte("bench payload")
	tr := &stubTransport{do: func(r *http.Request) (int, string, http.Header, error) {
		if r.URL.Query().Get("list-type") == "2" {
			return http.StatusOK, listHit(testID + ".txt"), nil, nil
		}
		return http.StatusOK, string(body), http.Header{"Content-Length": []string{itoa(int64(len(body)))}}, nil
	}}
	d := benchDriver(b, tr)
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := d.Download(ctx, testID); err != nil {
			b.Fatalf("Download err = %v", err)
		}
	}
}

func BenchmarkPresignURL(b *testing.B) {
	d := benchDriver(b, &stubTransport{})
	ctx := b.Context()
	key := testID + ".txt"
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		url, err := d.assetURL(ctx, key)
		if err != nil {
			b.Fatalf("assetURL err = %v", err)
		}
		if url == "" {
			b.Fatal("assetURL returned empty URL")
		}
	}
}

func BenchmarkFindKey(b *testing.B) {
	d := benchDriver(b, &stubTransport{do: func(*http.Request) (int, string, http.Header, error) {
		return http.StatusOK, listHit(testID + ".txt"), nil, nil
	}})
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		key, err := d.findKey(ctx, testID)
		if err != nil {
			b.Fatalf("findKey err = %v", err)
		}
		if key != testID+".txt" {
			b.Fatalf("findKey = %q, want %q", key, testID+".txt")
		}
	}
}

func BenchmarkIsNoSuchKey(b *testing.B) {
	err := stubAPIError{code: "NoSuchKey", msg: "gone"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if !isNoSuchKey(err) {
			b.Fatal("isNoSuchKey = false, want true")
		}
	}
}
