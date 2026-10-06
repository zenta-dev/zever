package storagetest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/storage"
)

// stubReporter records conformance reports instead of failing a test, so
// edge tests can drive every failure branch of the helpers.
type stubReporter struct {
	mu     sync.Mutex
	errors []string
	fatals []string
}

// Helper is a no-op.
func (r *stubReporter) Helper() {}

// Context returns a background context.
func (r *stubReporter) Context() context.Context { return context.Background() }

// Error records a report.
func (r *stubReporter) Error(args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, fmt.Sprint(args...))
}

// Errorf records a formatted report.
func (r *stubReporter) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

// Fatalf records a fatal report without stopping the caller.
func (r *stubReporter) Fatalf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fatals = append(r.fatals, fmt.Sprintf(format, args...))
}

// joined returns all recorded reports as one string.
func (r *stubReporter) joined() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(append(append([]string{}, r.errors...), r.fatals...), "\n")
}

// mustPresignArgs mirrors backend validation for the conformance stub.
func mustPresignArgs(bucket, key string, ttl time.Duration) error {
	if !storage.ValidBucket(bucket) {
		return fmt.Errorf("%w: %q", storage.ErrInvalidBucket, bucket)
	}

	if !storage.ValidKey(key) {
		return fmt.Errorf("%w: %q", storage.ErrInvalidKey, key)
	}

	if ttl <= 0 {
		return fmt.Errorf("%w: presign ttl must be positive, got %s", storage.ErrExpired, ttl)
	}

	if ttl > storage.MaxPresignTTL {
		return fmt.Errorf("%w: presign ttl %s exceeds max", storage.ErrPresignTTLExceeded, ttl)
	}

	return nil
}

// stubOKStorage is a conformance stub behaving like a correct backend.
type stubOKStorage struct{}

// PresignUpload mints a well-formed PUT URL or a validation error.
func (stubOKStorage) PresignUpload(_ context.Context, bucket, key, _ string, ttl time.Duration) (storage.PresignedURL, error) {
	if err := mustPresignArgs(bucket, key, ttl); err != nil {
		return storage.PresignedURL{}, err
	}

	return storage.PresignedURL{Method: storage.HTTPMethodPUT, URL: "https://example.com/" + bucket + "/" + key}, nil
}

// PresignDownload mints a well-formed GET URL or a validation error.
func (stubOKStorage) PresignDownload(_ context.Context, bucket, key string, ttl time.Duration) (storage.PresignedURL, error) {
	if err := mustPresignArgs(bucket, key, ttl); err != nil {
		return storage.PresignedURL{}, err
	}

	return storage.PresignedURL{Method: storage.HTTPMethodGET, URL: "https://example.com/" + bucket + "/" + key}, nil
}

// Exists reports absent for valid names and validation errors otherwise.
func (stubOKStorage) Exists(_ context.Context, bucket, key string) (bool, error) {
	if !storage.ValidBucket(bucket) {
		return false, fmt.Errorf("%w: %q", storage.ErrInvalidBucket, bucket)
	}

	if !storage.ValidKey(key) {
		return false, fmt.Errorf("%w: %q", storage.ErrInvalidKey, key)
	}

	return false, nil
}

// Delete is a no-op for valid names and a validation error otherwise.
func (stubOKStorage) Delete(_ context.Context, bucket, key string) error {
	if !storage.ValidBucket(bucket) {
		return fmt.Errorf("%w: %q", storage.ErrInvalidBucket, bucket)
	}

	if !storage.ValidKey(key) {
		return fmt.Errorf("%w: %q", storage.ErrInvalidKey, key)
	}

	return nil
}

// Move is a no-op onto itself, ErrNotFound for missing sources, and a
// validation error for bad names.
func (stubOKStorage) Move(_ context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	if srcBucket == dstBucket && srcKey == dstKey {
		return nil
	}

	if !storage.ValidBucket(srcBucket) {
		return fmt.Errorf("%w: %q", storage.ErrInvalidBucket, srcBucket)
	}

	if !storage.ValidKey(srcKey) {
		return fmt.Errorf("%w: %q", storage.ErrInvalidKey, srcKey)
	}

	if !storage.ValidBucket(dstBucket) {
		return fmt.Errorf("%w: %q", storage.ErrInvalidBucket, dstBucket)
	}

	if !storage.ValidKey(dstKey) {
		return fmt.Errorf("%w: %q", storage.ErrInvalidKey, dstKey)
	}

	return fmt.Errorf("%w: %s/%s", storage.ErrNotFound, srcBucket, srcKey)
}

// Close releases nothing.
func (stubOKStorage) Close(context.Context) error { return nil }

// Name returns the stub adapter name.
func (stubOKStorage) Name() string { return "stub" }

// mustContain fails the test when reports lack any want phrase.
func mustContain(t *testing.T, name, joined string, wants ...string) {
	t.Helper()

	for _, want := range wants {
		if !strings.Contains(joined, want) {
			t.Errorf("%s: reports lack %q:\n%s", name, want, joined)
		}
	}
}

// TestConformanceChecksOK pins that a correct backend produces no reports.
func TestConformanceChecksOK(t *testing.T) {
	t.Parallel()

	s := stubOKStorage{}
	r := &stubReporter{}

	conformancePresignUpload(r, s)
	conformancePresignDownload(r, s)
	conformanceExists(r, s)
	conformanceDelete(r, s)
	conformanceMove(r, s)
	conformanceClose(r, s)

	if got := r.joined(); got != "" {
		t.Fatalf("correct backend produced reports:\n%s", got)
	}
}

// TestConformancePresignFailures pins that each presign defect is reported on
// both paths: transport error, wrong method, empty URL, URL missing bucket
// or key, and lax validation accepting bad inputs.
func TestConformancePresignFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		run   func(r reporter, s storage.Storage)
		stub  storage.Storage
		wants []string
	}{
		{"upload error", conformancePresignUpload, stubErrUploadStorage{}, []string{"PresignUpload() error"}},
		{"upload method", conformancePresignUpload, stubMethodUploadStorage{}, []string{"want PUT"}},
		{"upload empty url", conformancePresignUpload, stubEmptyURLUploadStorage{}, []string{"URL is empty"}},
		{"upload no bucket", conformancePresignUpload, stubNoBucketURLUploadStorage{}, []string{"mention the bucket"}},
		{"upload no key", conformancePresignUpload, stubNoKeyURLUploadStorage{}, []string{"mention the key"}},
		{"upload lax", conformancePresignUpload, stubLaxUploadStorage{}, []string{"empty bucket", "oversize ttl"}},
		{"download error", conformancePresignDownload, stubErrDownloadStorage{}, []string{"PresignDownload() error"}},
		{"download method", conformancePresignDownload, stubMethodDownloadStorage{}, []string{"want GET"}},
		{"download empty url", conformancePresignDownload, stubEmptyURLDownloadStorage{}, []string{"URL is empty"}},
		{"download no bucket", conformancePresignDownload, stubNoBucketURLDownloadStorage{}, []string{"mention the bucket"}},
		{"download no key", conformancePresignDownload, stubNoKeyURLDownloadStorage{}, []string{"mention the key"}},
		{"download lax", conformancePresignDownload, stubLaxDownloadStorage{}, []string{"empty bucket", "oversize ttl"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := &stubReporter{}
			tc.run(r, tc.stub)
			mustContain(t, tc.name, r.joined(), tc.wants...)
		})
	}
}

// TestConformanceExistsFailures pins the absent-object and validation
// reports for Exists.
func TestConformanceExistsFailures(t *testing.T) {
	t.Parallel()

	r := &stubReporter{}
	conformanceExists(r, stubFoundExistsStorage{})
	mustContain(t, "found", r.joined(), "want false,nil", "ErrInvalidBucket", "ErrInvalidKey")

	r = &stubReporter{}
	conformanceExists(r, stubErrExistsStorage{})
	if got := r.joined(); !strings.Contains(got, "Exists(missing)") {
		t.Errorf("error: reports lack missing-object phrase:\n%s", got)
	}
}

// TestConformanceDeleteFailures pins that a failing Delete is reported,
// including validation errors.
func TestConformanceDeleteFailures(t *testing.T) {
	t.Parallel()

	r := &stubReporter{}
	conformanceDelete(r, stubErrDeleteStorage{})
	mustContain(t, "delete", r.joined(), "want nil", "ErrInvalidBucket", "ErrInvalidKey")
}

// TestConformanceMoveFailures pins the self-move, missing-source, and
// validation reports for Move.
func TestConformanceMoveFailures(t *testing.T) {
	t.Parallel()

	r := &stubReporter{}
	conformanceMove(r, stubErrMoveStorage{})
	mustContain(t, "move", r.joined(), "want nil", "ErrNotFound", "ErrInvalidBucket", "ErrInvalidKey")
}

// TestConformanceCloseFailures pins the empty-name, first-close, and
// second-close reports.
func TestConformanceCloseFailures(t *testing.T) {
	t.Parallel()

	r := &stubReporter{}
	conformanceClose(r, stubEmptyNameStorage{})
	mustContain(t, "empty name", r.joined(), "Name() is empty")

	r = &stubReporter{}
	conformanceClose(r, stubErrCloseStorage{})
	mustContain(t, "close error", r.joined(), "Close() error", "second")

	r = &stubReporter{}
	conformanceClose(r, newStubSecondErrCloseStorage())
	got := r.joined()
	mustContain(t, "second close", got, "second")
	if strings.Contains(got, "Close() error =") {
		t.Errorf("second close: reports unexpected first-close failure:\n%s", got)
	}
}

// TestConformanceChecksConcurrent pins that the helpers are safe for
// concurrent use with independent reporters.
func TestConformanceChecksConcurrent(t *testing.T) {
	t.Parallel()

	s := stubOKStorage{}
	r := &stubReporter{}

	var wg sync.WaitGroup
	for _, run := range []func(reporter, storage.Storage){
		conformancePresignUpload,
		conformancePresignDownload,
		conformanceExists,
		conformanceDelete,
		conformanceMove,
		conformanceClose,
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run(r, s)
		}()
	}

	wg.Wait()

	if got := r.joined(); got != "" {
		t.Fatalf("concurrent checks produced reports:\n%s", got)
	}
}

// stubErrUploadStorage fails every upload.
type stubErrUploadStorage struct{ stubOKStorage }

// PresignUpload returns a transport error.
func (stubErrUploadStorage) PresignUpload(context.Context, string, string, string, time.Duration) (storage.PresignedURL, error) {
	return storage.PresignedURL{}, errors.New("stub: upload boom")
}

// stubMethodUploadStorage mints the wrong method.
type stubMethodUploadStorage struct{ stubOKStorage }

// PresignUpload returns a download-shaped URL.
func (stubMethodUploadStorage) PresignUpload(_ context.Context, bucket, key, _ string, ttl time.Duration) (storage.PresignedURL, error) {
	if err := mustPresignArgs(bucket, key, ttl); err != nil {
		return storage.PresignedURL{}, err
	}

	return storage.PresignedURL{Method: storage.HTTPMethodGET, URL: "https://example.com/" + bucket + "/" + key}, nil
}

// stubEmptyURLUploadStorage mints an empty URL.
type stubEmptyURLUploadStorage struct{ stubOKStorage }

// PresignUpload returns a method with no URL.
func (stubEmptyURLUploadStorage) PresignUpload(_ context.Context, bucket, key, _ string, ttl time.Duration) (storage.PresignedURL, error) {
	if err := mustPresignArgs(bucket, key, ttl); err != nil {
		return storage.PresignedURL{}, err
	}

	return storage.PresignedURL{Method: storage.HTTPMethodPUT}, nil
}

// stubNoBucketURLUploadStorage mints a URL that drops the bucket.
type stubNoBucketURLUploadStorage struct{ stubOKStorage }

// PresignUpload returns a URL without the bucket name.
func (stubNoBucketURLUploadStorage) PresignUpload(_ context.Context, bucket, key, _ string, ttl time.Duration) (storage.PresignedURL, error) {
	if err := mustPresignArgs(bucket, key, ttl); err != nil {
		return storage.PresignedURL{}, err
	}

	return storage.PresignedURL{Method: storage.HTTPMethodPUT, URL: "https://example.com/" + key}, nil
}

// stubNoKeyURLUploadStorage mints a URL that drops the key.
type stubNoKeyURLUploadStorage struct{ stubOKStorage }

// PresignUpload returns a URL without the key.
func (stubNoKeyURLUploadStorage) PresignUpload(_ context.Context, bucket, key, _ string, ttl time.Duration) (storage.PresignedURL, error) {
	if err := mustPresignArgs(bucket, key, ttl); err != nil {
		return storage.PresignedURL{}, err
	}

	return storage.PresignedURL{Method: storage.HTTPMethodPUT, URL: "https://example.com/" + bucket + "/other.txt"}, nil
}

// stubLaxUploadStorage accepts every input.
type stubLaxUploadStorage struct{ stubOKStorage }

// PresignUpload never validates.
func (stubLaxUploadStorage) PresignUpload(_ context.Context, bucket, key, _ string, _ time.Duration) (storage.PresignedURL, error) {
	return storage.PresignedURL{Method: storage.HTTPMethodPUT, URL: "https://example.com/" + bucket + "/" + key}, nil
}

// stubErrDownloadStorage fails every download.
type stubErrDownloadStorage struct{ stubOKStorage }

// PresignDownload returns a transport error.
func (stubErrDownloadStorage) PresignDownload(context.Context, string, string, time.Duration) (storage.PresignedURL, error) {
	return storage.PresignedURL{}, errors.New("stub: download boom")
}

// stubMethodDownloadStorage mints the wrong method.
type stubMethodDownloadStorage struct{ stubOKStorage }

// PresignDownload returns an upload-shaped URL.
func (stubMethodDownloadStorage) PresignDownload(_ context.Context, bucket, key string, ttl time.Duration) (storage.PresignedURL, error) {
	if err := mustPresignArgs(bucket, key, ttl); err != nil {
		return storage.PresignedURL{}, err
	}

	return storage.PresignedURL{Method: storage.HTTPMethodPUT, URL: "https://example.com/" + bucket + "/" + key}, nil
}

// stubEmptyURLDownloadStorage mints an empty URL.
type stubEmptyURLDownloadStorage struct{ stubOKStorage }

// PresignDownload returns a method with no URL.
func (stubEmptyURLDownloadStorage) PresignDownload(_ context.Context, bucket, key string, ttl time.Duration) (storage.PresignedURL, error) {
	if err := mustPresignArgs(bucket, key, ttl); err != nil {
		return storage.PresignedURL{}, err
	}

	return storage.PresignedURL{Method: storage.HTTPMethodGET}, nil
}

// stubNoBucketURLDownloadStorage mints a URL that drops the bucket.
type stubNoBucketURLDownloadStorage struct{ stubOKStorage }

// PresignDownload returns a URL without the bucket name.
func (stubNoBucketURLDownloadStorage) PresignDownload(_ context.Context, bucket, key string, ttl time.Duration) (storage.PresignedURL, error) {
	if err := mustPresignArgs(bucket, key, ttl); err != nil {
		return storage.PresignedURL{}, err
	}

	return storage.PresignedURL{Method: storage.HTTPMethodGET, URL: "https://example.com/" + key}, nil
}

// stubNoKeyURLDownloadStorage mints a URL that drops the key.
type stubNoKeyURLDownloadStorage struct{ stubOKStorage }

// PresignDownload returns a URL without the key.
func (stubNoKeyURLDownloadStorage) PresignDownload(_ context.Context, bucket, key string, ttl time.Duration) (storage.PresignedURL, error) {
	if err := mustPresignArgs(bucket, key, ttl); err != nil {
		return storage.PresignedURL{}, err
	}

	return storage.PresignedURL{Method: storage.HTTPMethodGET, URL: "https://example.com/" + bucket + "/other.txt"}, nil
}

// stubLaxDownloadStorage accepts every input.
type stubLaxDownloadStorage struct{ stubOKStorage }

// PresignDownload never validates.
func (stubLaxDownloadStorage) PresignDownload(_ context.Context, bucket, key string, _ time.Duration) (storage.PresignedURL, error) {
	return storage.PresignedURL{Method: storage.HTTPMethodGET, URL: "https://example.com/" + bucket + "/" + key}, nil
}

// stubFoundExistsStorage reports every object present.
type stubFoundExistsStorage struct{ stubOKStorage }

// Exists reports every valid object present and drops validation errors,
// simulating a backend that skips input checks.
func (stubFoundExistsStorage) Exists(_ context.Context, bucket, key string) (bool, error) {
	if !storage.ValidBucket(bucket) || !storage.ValidKey(key) {
		return false, nil
	}

	return true, nil
}

// stubErrExistsStorage fails every existence check.
type stubErrExistsStorage struct{ stubOKStorage }

// Exists returns a transport error.
func (stubErrExistsStorage) Exists(context.Context, string, string) (bool, error) {
	return false, errors.New("stub: exists boom")
}

// stubErrDeleteStorage fails every delete.
type stubErrDeleteStorage struct{ stubOKStorage }

// Delete returns a transport error.
func (stubErrDeleteStorage) Delete(context.Context, string, string) error {
	return errors.New("stub: delete boom")
}

// stubErrMoveStorage fails every move.
type stubErrMoveStorage struct{ stubOKStorage }

// Move returns a transport error.
func (stubErrMoveStorage) Move(context.Context, string, string, string, string) error {
	return errors.New("stub: move boom")
}

// stubEmptyNameStorage reports an empty adapter name.
type stubEmptyNameStorage struct{ stubOKStorage }

// Name returns an empty name.
func (stubEmptyNameStorage) Name() string { return "" }

// stubErrCloseStorage fails every close.
type stubErrCloseStorage struct{ stubOKStorage }

// Close returns a transport error.
func (stubErrCloseStorage) Close(context.Context) error {
	return errors.New("stub: close boom")
}

// stubSecondErrCloseStorage fails only the second close.
type stubSecondErrCloseStorage struct {
	stubOKStorage
	calls int
}

// newStubSecondErrCloseStorage returns a close stub failing on second use.
func newStubSecondErrCloseStorage() *stubSecondErrCloseStorage {
	return &stubSecondErrCloseStorage{}
}

// Close succeeds once, then fails.
func (s *stubSecondErrCloseStorage) Close(context.Context) error {
	s.calls++
	if s.calls == 1 {
		return nil
	}

	return errors.New("stub: second close boom")
}
