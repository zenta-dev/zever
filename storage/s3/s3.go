package s3

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/internal/s3opts"
	"github.com/zenta-dev/zever/storage"
	"github.com/zenta-dev/zever/storage/s3core"
)

type s3Adapter struct {
	*s3core.Core
	policySync string
	syncFail   string
}

// New validates opts, defaults empty region to us-east-1, requires url_base when endpoint is set, resolves policy_sync (manual or auto, default manual) and sync_fail (warn or require, default warn), and checks policy coherence.
// New builds the client via s3opts.NewClient (shared helper over s3core) and returns the storage.Storage adapter.
// New returns an error for invalid options, incoherent policy, or client creation failure.
func New(opts storage.Options) (storage.Storage, error) {
	const prefix = "s3"

	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}

	cfg := s3opts.Config{
		Endpoint:        opts.Endpoint,
		Region:          opts.Region,
		AccessKeyID:     opts.AccessKeyID,
		SecretAccessKey: opts.SecretAccessKey,
	}.WithDefaults(s3opts.DefaultRegion)

	region := cfg.Region
	endpoint := cfg.Endpoint
	urlBase := opts.URLBase

	if endpoint != "" && urlBase == "" {
		return nil, fmt.Errorf(
			"%w: option %q must be set when %q is configured "+
				"(custom endpoints must not emit amazonaws URLs)",
			ErrInvalidOption, "url_base", "endpoint",
		)
	}

	var store storage.PolicyStore
	// opts.Validate above already resolved the same config; this cannot fail.
	_ = store.ResolveFromConfig(opts.Policy)

	policySync := opts.PolicySync
	if policySync == "" {
		policySync = "manual"
	}

	if policySync != "manual" && policySync != "auto" {
		return nil, fmt.Errorf("%w: option %q must be %q or %q, got %q", ErrInvalidOption, "policy_sync", "manual", "auto", policySync)
	}

	syncFail := opts.SyncFail
	if syncFail == "" {
		syncFail = "warn"
	}

	if syncFail != "warn" && syncFail != "require" {
		return nil, fmt.Errorf("%w: option %q must be %q or %q, got %q", ErrInvalidOption, "sync_fail", "warn", "require", syncFail)
	}

	if store.Configured() {
		if err := s3core.ValidatePolicyCoherence(prefix, store.Policies(), store.Default()); err != nil {
			return nil, fmt.Errorf("s3: %w", err)
		}
	}

	client, presigner, err := s3opts.NewClient(context.Background(), prefix, cfg, urlBase)
	if err != nil {
		return nil, fmt.Errorf("s3: %w", err)
	}

	return &s3Adapter{
		Core:       s3core.New(prefix, region, urlBase, client, presigner, store, nil),
		policySync: policySync,
		syncFail:   syncFail,
	}, nil
}

func (a *s3Adapter) Close(context.Context) error {
	return nil
}

func (a *s3Adapter) Name() string {
	return "s3"
}
