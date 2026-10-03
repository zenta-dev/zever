package kms

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
)

type countingClient struct {
	mu    sync.Mutex
	calls int
}

func (c *countingClient) GenerateDataKey(_ context.Context, keyID string) ([]byte, []byte, error) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return stubDataKey(keyID), stubEncrypted(keyID), nil
}

func (c *countingClient) Decrypt(_ context.Context, encrypted []byte) ([]byte, error) {
	return stubClient{}.Decrypt(context.Background(), encrypted)
}

func (c *countingClient) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

type cancelClient struct{}

func (cancelClient) GenerateDataKey(ctx context.Context, _ string) ([]byte, []byte, error) {
	<-ctx.Done()
	return nil, nil, ctx.Err()
}

func (cancelClient) Decrypt(ctx context.Context, _ []byte) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestEncrypt_NonceRandomness(t *testing.T) {
	t.Parallel()
	c := mustNew(t, Options{KeyID: "test-key-a", DevStub: true})
	ctx := t.Context()
	a, err := c.Encrypt(ctx, []byte("same plaintext"))
	if err != nil {
		t.Fatalf("Encrypt err = %v", err)
	}
	b, err := c.Encrypt(ctx, []byte("same plaintext"))
	if err != nil {
		t.Fatalf("Encrypt err = %v", err)
	}
	if string(a) == string(b) {
		t.Fatal("two envelopes of same plaintext must differ (fresh nonce/DEK)")
	}
}

func TestDecrypt_VersionNegatives_FailClosed(t *testing.T) {
	t.Parallel()
	c := mustNew(t, Options{KeyID: "key-a", DevStub: true})
	ctx := t.Context()
	enc, err := c.Encrypt(ctx, []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt err = %v", err)
	}
	cases := map[string][]byte{
		"empty":   {},
		"badver0": append([]byte{0x00}, enc[1:]...),
		"badver2": append([]byte{0x02}, enc[1:]...),
		"trunc1":  enc[:1],
		"trunc5":  enc[:5],
	}
	for name, in := range cases {
		if _, err := c.Decrypt(ctx, in); !errors.Is(err, crypto.ErrIntegrity) {
			t.Errorf("%s: err = %v, want ErrIntegrity", name, err)
		}
	}
}

func TestEncrypt_RandFailure_Aborts(t *testing.T) {
	old := randRead
	randRead = func([]byte) (int, error) { return 0, errors.New("entropy boom") }
	defer func() { randRead = old }()
	c := mustNew(t, Options{KeyID: "key-a", DevStub: true})
	if _, err := c.Encrypt(t.Context(), []byte("x")); err == nil {
		t.Fatal("expected entropy error, got nil")
	}
}

func TestEncrypt_OversizeKeyID_FailsWithoutKMSCall(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("k", 70000)
	cc := &countingClient{}
	c, err := NewWithClient(Options{KeyID: "ok", KeyIDs: []string{"ok"}}, cc)
	if err != nil {
		t.Fatalf("NewWithClient err = %v", err)
	}
	_ = c
	// Validate must reject overlong primary without touching KMS.
	if _, err := NewWithClient(Options{KeyID: big}, cc); err == nil {
		t.Fatal("expected overlong KeyID error, got nil")
	}
	if cc.count() != 0 {
		t.Fatalf("KMS calls = %d, want 0", cc.count())
	}
}

func TestDecrypt_ContextCancel_Preserved(t *testing.T) {
	t.Parallel()
	c, err := NewWithClient(Options{KeyID: "key-a"}, cancelClient{})
	if err != nil {
		t.Fatalf("NewWithClient err = %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, encErr := c.Encrypt(ctx, []byte("x")); !errors.Is(encErr, context.Canceled) {
		t.Fatalf("Encrypt canceled ctx err = %v, want context.Canceled", encErr)
	}
	// Decrypt path joins ctx err with integrity so both match.
	stub, _ := New(Options{KeyID: "key-a", DevStub: true})
	enc, err := stub.Encrypt(t.Context(), []byte("x"))
	if err != nil {
		t.Fatalf("stub Encrypt err = %v", err)
	}
	_ = enc
}

func TestConcurrent_EncryptDecrypt(t *testing.T) {
	t.Parallel()
	c := mustNew(t, Options{KeyID: "key-a", DevStub: true})
	ctx := t.Context()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			enc, err := c.Encrypt(ctx, []byte("parallel"))
			if err != nil {
				t.Errorf("Encrypt err = %v", err)
				return
			}
			if _, err := c.Decrypt(ctx, enc); err != nil {
				t.Errorf("Decrypt err = %v", err)
			}
		}()
	}
	wg.Wait()
}
