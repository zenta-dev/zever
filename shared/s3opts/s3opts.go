// Package s3opts provides a shared typed S3 credential and endpoint helper
// for media and storage backends. It validates bucket, credentials, region
// defaults, and endpoint scheme/host, and builds S3 clients via NewCoreClient
// without extra network calls. Secrets are never included in errors.
package s3opts

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const DefaultRegion = "us-east-1"

var (
	ErrMissingBucket      = errors.New("s3opts: bucket is required")
	ErrMissingCredentials = errors.New("s3opts: access key and secret are required")
	ErrInvalidEndpoint    = errors.New("s3opts: invalid endpoint")
	ErrInvalidRegion      = errors.New("s3opts: invalid region")
)

type Options struct {
	Endpoint        string `json:"endpoint" toml:"endpoint" yaml:"endpoint"`
	Region          string `json:"region" toml:"region" yaml:"region"`
	Bucket          string `json:"bucket" toml:"bucket" yaml:"bucket"`
	AccessKeyID     string `json:"access_key_id" toml:"access_key_id" yaml:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key" toml:"secret_access_key" yaml:"secret_access_key"`
}

// Config aliases Options for compatibility.
type Config = Options

func (o Options) WithDefaults(defaultRegion string) Options {
	if defaultRegion == "" {
		defaultRegion = DefaultRegion
	}
	out := Options{
		Endpoint:        strings.TrimSpace(o.Endpoint),
		Region:          strings.TrimSpace(o.Region),
		Bucket:          strings.TrimSpace(o.Bucket),
		AccessKeyID:     strings.TrimSpace(o.AccessKeyID),
		SecretAccessKey: strings.TrimSpace(o.SecretAccessKey),
	}
	if out.Region == "" {
		out.Region = defaultRegion
	}
	return out
}

func (o Options) ValidateBucket() error {
	if strings.TrimSpace(o.Bucket) == "" {
		return ErrMissingBucket
	}
	return nil
}

func (o Options) ValidateCredentials() error {
	if strings.TrimSpace(o.AccessKeyID) == "" || strings.TrimSpace(o.SecretAccessKey) == "" {
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

func (o Options) ValidateEndpoint() error {
	v := strings.TrimSpace(o.Endpoint)
	if v == "" {
		return nil
	}
	return validateURL(v)
}

func (o Options) ValidateRegion() error {
	r := strings.TrimSpace(o.Region)
	if r == "" {
		return fmt.Errorf("%w: region is required", ErrInvalidRegion)
	}
	if strings.Contains(r, " ") || strings.Contains(r, "\n") || strings.Contains(r, "\t") {
		return fmt.Errorf("%w: region must not contain whitespace", ErrInvalidRegion)
	}
	return nil
}

func (o Options) Validate(requireBucket, requireCredentials bool) error {
	var errs []error
	if err := o.ValidateEndpoint(); err != nil {
		errs = append(errs, err)
	}
	if err := o.ValidateRegion(); err != nil {
		errs = append(errs, err)
	}
	if requireBucket {
		if err := o.ValidateBucket(); err != nil {
			errs = append(errs, err)
		}
	}
	if requireCredentials {
		if err := o.ValidateCredentials(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

var coreNewClient = NewCoreClient

func NewClient(ctx context.Context, prefix string, cfg Options, urlBase string) (*s3.Client, *s3.PresignClient, error) {
	region := cfg.Region
	if region == "" {
		region = DefaultRegion
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	trimBase := strings.TrimSpace(urlBase)
	check := Options{Endpoint: endpoint, Region: region}
	if err := check.ValidateEndpoint(); err != nil {
		return nil, nil, fmt.Errorf("%s: endpoint: %w", prefix, err)
	}
	regCheck := Options{Region: region}
	if err := regCheck.ValidateRegion(); err != nil {
		return nil, nil, fmt.Errorf("%s: region: %w", prefix, err)
	}
	if trimBase != "" {
		if err := validateURL(trimBase); err != nil {
			return nil, nil, fmt.Errorf("%s: url_base: %w", prefix, err)
		}
	}
	client, presigner, err := coreNewClient(ctx, prefix, region, endpoint, trimBase, strings.TrimSpace(cfg.AccessKeyID), strings.TrimSpace(cfg.SecretAccessKey))
	if err != nil {
		return nil, nil, err
	}
	return client, presigner, nil
}
