package serve

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/core/crypto"
)

// loadTLS builds the shared TLS configuration for the HTTP and gRPC
// listeners from the router battery's tls_cert/tls_key/tls_client_ca
// options. It returns nil when no TLS paths are set: both listeners stay
// plaintext. The floor is TLS 1.2; tls_client_ca turns on mutual TLS.
func loadTLS(cfg *config.Config) (*tls.Config, error) {
	certFile := cfg.Router.Options.TLSCertFile
	keyFile := cfg.Router.Options.TLSKeyFile
	clientCAFile := cfg.Router.Options.TLSClientCAFile

	if certFile == "" && keyFile == "" && clientCAFile == "" {
		return nil, nil //nolint:nilnil // nil means "TLS off"; callers branch on it.
	}

	if certFile == "" || keyFile == "" {
		return nil, errors.New("serve: tls_cert and tls_key must be set together (ROUTER_TLS_CERT/ROUTER_TLS_KEY)")
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("serve: load TLS key pair (ROUTER_TLS_CERT/ROUTER_TLS_KEY): %w", err)
	}

	tlsCfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
	}

	if clientCAFile != "" {
		pem, err := os.ReadFile(clientCAFile)
		if err != nil {
			return nil, fmt.Errorf("serve: read TLS client CA (ROUTER_TLS_CLIENT_CA): %w", err)
		}

		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, errors.New("serve: no valid certificates in TLS client CA file (ROUTER_TLS_CLIENT_CA)")
		}

		tlsCfg.ClientCAs = pool
		tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return tlsCfg, nil
}

// reflectionEnabled reports whether the gRPC server should register server
// reflection. Secure-by-default: off unless the operator sets
// GRPC_REFLECTION=true (an explicit false always wins, even in dev) or the
// process runs in a dev ZEVER_ENV, where reflection is a local convenience.
func reflectionEnabled() bool {
	if v, ok := lookupEnv("GRPC_REFLECTION"); ok {
		// Explicit setting wins; unparseable values fail closed to off.
		return v == "true" || v == "1"
	}

	if v, ok := lookupEnv("ZEVER_ENV"); ok {
		return crypto.IsDevEnv(v)
	}

	return false
}
