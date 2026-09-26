package s3core

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/zenta-dev/zever/storage"
)

// Core is shared S3-compatible state used by s3 and r2 adapters. Unexported prefix and staticURL customize endpoint scoping and public URL generation.
type Core struct {
	// PolicyStore provides bucket policy lookup for authorization checks.
	storage.PolicyStore
	// Region is AWS region used for signing and default URL generation.
	Region string
	// URLBase overrides default public URL base for StaticURL generation.
	URLBase string
	// Client is configured S3 API client used for storage operations.
	Client *s3.Client
	// Presigner generates presigned S3 URLs for uploads and downloads.
	Presigner *s3.PresignClient

	prefix    string
	staticURL func(bucket, key string) (string, error)
}

// New builds a Core from prefix, region, urlBase, client, presigner, store, and staticURL. StaticURL defaults to the StaticURL method when staticURL is nil.
func New(
	prefix, region, urlBase string,
	client *s3.Client,
	presigner *s3.PresignClient,
	store storage.PolicyStore,
	staticURL func(bucket, key string) (string, error),
) *Core {
	c := &Core{
		PolicyStore: store,
		Region:      region,
		URLBase:     urlBase,
		Client:      client,
		Presigner:   presigner,
		prefix:      prefix,
		staticURL:   staticURL,
	}
	if c.staticURL == nil {
		c.staticURL = c.StaticURL
	}

	return c
}

// NewClient builds an S3 client and presigner from endpoint and credential settings. It validates endpoint and url_base, loads AWS config with static credentials, and sets BaseEndpoint with path-style addressing when a base is set.
func NewClient(ctx context.Context, prefix, region, endpoint, urlBase, accessKey, secretKey string) (*s3.Client, *s3.PresignClient, error) {
	if err := storage.ValidateBaseURL(prefix, "endpoint", endpoint); err != nil {
		return nil, nil, err
	}

	if err := storage.ValidateBaseURL(prefix, "url_base", urlBase); err != nil {
		return nil, nil, err
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: load config: %w", prefix, err)
	}

	base := urlBase
	if base == "" {
		base = endpoint
	}

	if base != "" {
		cfg.BaseEndpoint = aws.String(base)

		client := s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.UsePathStyle = true
		})

		return client, s3.NewPresignClient(client), nil
	}

	client := s3.NewFromConfig(cfg)

	return client, s3.NewPresignClient(client), nil
}
