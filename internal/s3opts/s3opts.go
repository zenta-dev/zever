// Package s3opts provides a shared typed S3 credential and endpoint helper
// for media and storage backends. It validates bucket, credentials, region
// defaults, and endpoint scheme/host, and builds S3 clients via s3core
// without extra network calls. Secrets are never included in errors.
package s3opts

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/zenta-dev/zever/storage/s3core"
)

const DefaultRegion = "us-east-1"

var (
	ErrMissingBucket      = errors.New("s3opts: bucket is required")
	ErrMissingCredentials = errors.New("s3opts: access key and secret are required")
	ErrInvalidEndpoint    = errors.New("s3opts: invalid endpoint")
	ErrInvalidRegion      = errors.New("s3opts: invalid region")
)

type Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
}

func (c Config) WithDefaults(defaultRegion string) Config {
	if defaultRegion == "" {
		defaultRegion = DefaultRegion
	}
	out := Config{
		Endpoint:        strings.TrimSpace(c.Endpoint),
		Region:          strings.TrimSpace(c.Region),
		Bucket:          strings.TrimSpace(c.Bucket),
		AccessKeyID:     strings.TrimSpace(c.AccessKeyID),
		SecretAccessKey: strings.TrimSpace(c.SecretAccessKey),
	}
	if out.Region == "" {
		out.Region = defaultRegion
	}
	return out
}

func (c Config) ValidateBucket() error {
	if strings.TrimSpace(c.Bucket) == "" {
		return ErrMissingBucket
	}
	return nil
}

func (c Config) ValidateCredentials() error {
	if strings.TrimSpace(c.AccessKeyID) == "" || strings.TrimSpace(c.SecretAccessKey) == "" {
		return ErrMissingCredentials
	}
	return nil
}

func validateURL(value string) error {
	if value == "" {
		return nil
	}
	if strings.Contains(value, " ") || strings.Contains(value, "\n") || strings.Contains(value, "\t") {
		return fmt.Errorf("%w: URL must not contain whitespace", ErrInvalidEndpoint)
	}
	u, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%w: URL must be a valid URL: %w", ErrInvalidEndpoint, err)
	}
	if u.Scheme == "" {
		return fmt.Errorf("%w: URL must include scheme", ErrInvalidEndpoint)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: URL must include host", ErrInvalidEndpoint)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: URL scheme must be http or https, got %q", ErrInvalidEndpoint, u.Scheme)
	}
	if u.User != nil {
		return fmt.Errorf("%w: URL must not contain user info", ErrInvalidEndpoint)
	}
	return nil
}

func (c Config) ValidateEndpoint() error {
	v := strings.TrimSpace(c.Endpoint)
	if v == "" {
		return nil
	}
	return validateURL(v)
}

func (c Config) ValidateRegion() error {
	r := strings.TrimSpace(c.Region)
	if r == "" {
		return fmt.Errorf("%w: region is required", ErrInvalidRegion)
	}
	if strings.Contains(r, " ") || strings.Contains(r, "\n") || strings.Contains(r, "\t") {
		return fmt.Errorf("%w: region must not contain whitespace", ErrInvalidRegion)
	}
	return nil
}

func (c Config) Validate(requireBucket, requireCredentials bool) error {
	var errs []error
	if err := c.ValidateEndpoint(); err != nil {
		errs = append(errs, err)
	}
	if err := c.ValidateRegion(); err != nil {
		errs = append(errs, err)
	}
	if requireBucket {
		if err := c.ValidateBucket(); err != nil {
			errs = append(errs, err)
		}
	}
	if requireCredentials {
		if err := c.ValidateCredentials(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

var coreNewClient = s3core.NewClient

func NewClient(ctx context.Context, prefix string, cfg Config, urlBase string) (*s3.Client, *s3.PresignClient, error) {
	region := cfg.Region
	if region == "" {
		region = DefaultRegion
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	trimBase := strings.TrimSpace(urlBase)
	check := Config{Endpoint: endpoint, Region: region}
	if err := check.ValidateEndpoint(); err != nil {
		return nil, nil, fmt.Errorf("[%s] endpoint: %w", prefix, err)
	}
	regCheck := Config{Region: region}
	if err := regCheck.ValidateRegion(); err != nil {
		return nil, nil, fmt.Errorf("[%s] region: %w", prefix, err)
	}
	if trimBase != "" {
		if err := validateURL(trimBase); err != nil {
			return nil, nil, fmt.Errorf("[%s] url_base: %w", prefix, err)
		}
	}
	client, presigner, err := coreNewClient(ctx, prefix, region, endpoint, trimBase, strings.TrimSpace(cfg.AccessKeyID), strings.TrimSpace(cfg.SecretAccessKey))
	if err != nil {
		return nil, nil, err
	}
	return client, presigner, nil
}
