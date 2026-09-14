package r2

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/zenta-dev/zever/storage"
	"github.com/zenta-dev/zever/storage/s3core"
)

const defaultRegion = "auto"

type r2Adapter struct {
	*s3core.Core
}

// New creates an R2 Storage backend from opts.
func New(opts storage.Options) (storage.Storage, error) {
	const prefix = "r2"

	if err := opts.Validate(); err != nil {
		return nil, err
	}

	accountID := strings.TrimSpace(opts.AccountID)
	if accountID == "" {
		return nil, fmt.Errorf("[r2] option %q is required", "account_id")
	}

	accessKey := strings.TrimSpace(opts.AccessKeyID)
	if accessKey == "" {
		return nil, fmt.Errorf("[r2] option %q is required", "access_key_id")
	}

	secretKey := strings.TrimSpace(opts.SecretAccessKey)
	if secretKey == "" {
		return nil, fmt.Errorf("[r2] option %q is required", "secret_access_key")
	}

	region := opts.Region
	if region == "" {
		region = defaultRegion
	}

	endpoint, err := r2Endpoint(accountID, opts.Endpoint)
	if err != nil {
		return nil, err
	}

	urlBase := opts.URLBase
	pubBase := opts.PublicURL

	var store storage.PolicyStore
	// opts.Validate above already resolved the same config; this cannot fail.
	_ = store.ResolveFromConfig(opts.Policy)

	if pubBase != "" {
		if err = storage.ValidateBaseURL(prefix, "public_url", pubBase); err != nil {
			return nil, err
		}
	}

	if store.Configured() {
		if err = checkR2PublicPolicy(store.Policies(), store.Default(), pubBase); err != nil {
			return nil, err
		}

		if err = s3core.ValidatePolicyCoherence(prefix, store.Policies(), store.Default()); err != nil {
			return nil, err
		}
	}

	client, presigner, err := s3core.NewClient(context.Background(), prefix, region, endpoint, urlBase, accessKey, secretKey)
	if err != nil {
		return nil, err
	}

	// New rejects public policies without pubBase above, so pubBase is always
	// set whenever this closure runs for a public permission.
	static := func(bucket, key string) (string, error) {
		return strings.TrimRight(pubBase, "/") + "/" + url.PathEscape(bucket) + "/" + s3core.EscapeKey(key), nil
	}

	a := &r2Adapter{
		Core: s3core.New(prefix, region, urlBase, client, presigner, store, static),
	}

	return a, nil
}

func (a *r2Adapter) Name() string {
	return "r2"
}

func isValidAccountID(s string) bool {
	if s == "" {
		return false
	}

	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}

		return false
	}

	return true
}

func r2Endpoint(accountID, endpoint string) (string, error) {
	if !isValidAccountID(accountID) {
		return "", fmt.Errorf("[r2] option %q must be alphanumeric, hyphen or underscore", "account_id")
	}

	if endpoint != "" {
		return endpoint, nil
	}

	return fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID), nil
}

func hasPublicPerm(p storage.Policy) bool {
	return p.Public(storage.PermRead) || p.Public(storage.PermWrite) || p.Public(storage.PermUpdate) || p.Public(storage.PermDelete)
}

func checkR2PublicPolicy(policies map[storage.BucketName]storage.Policy, def storage.Policy, pubBase string) error {
	if pubBase != "" {
		return nil
	}

	for bucket, p := range policies {
		if hasPublicPerm(p) {
			return fmt.Errorf(
				"[r2] policy for bucket %q has public permissions, requiring option %q "+
					"(R2 public bucket custom domain)",
				bucket, "public_url_base",
			)
		}
	}

	if hasPublicPerm(def) {
		return fmt.Errorf(
			"[r2] default policy has public permissions, requiring option %q "+
				"(R2 public bucket custom domain)",
			"public_url_base",
		)
	}

	return nil
}

func (a *r2Adapter) Close(context.Context) error {
	return nil
}
