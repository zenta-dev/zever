package cryptotest_test

import (
	"context"
	"testing"

	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/core/crypto/cryptotest"
)

// stubRemote is a KMS-like backend that keeps keys remotely: local
// Encrypt/Sign/Mac report ErrNotSupported and foreign bytes fail Decrypt
// with ErrIntegrity, exercising the kit's documented remote-key exemption.
type stubRemote struct{}

func (stubRemote) Encrypt(context.Context, []byte) ([]byte, error) {
	return nil, crypto.ErrNotSupported
}

func (stubRemote) Decrypt(context.Context, []byte) ([]byte, error) {
	return nil, crypto.ErrIntegrity
}

func (stubRemote) Sign(context.Context, []byte) ([]byte, error) {
	return nil, crypto.ErrNotSupported
}

func (stubRemote) Verify(context.Context, []byte, []byte) (bool, error) {
	return false, crypto.ErrNotSupported
}

func (stubRemote) Mac(context.Context, []byte) ([]byte, error) {
	return nil, crypto.ErrNotSupported
}

func (stubRemote) VerifyMac(context.Context, []byte, []byte) (bool, error) {
	return false, crypto.ErrNotSupported
}

var _ crypto.Crypto = stubRemote{}

// TestConformanceRemoteExempt proves the kit accepts the documented KMS
// exemption: adapters delegating key storage may report ErrNotSupported
// for local-key operations instead of round-tripping.
func TestConformanceRemoteExempt(t *testing.T) {
	t.Parallel()

	cryptotest.Conformance(t, func(t *testing.T) crypto.Crypto {
		t.Helper()

		return stubRemote{}
	})
}
