package cloudflare

import (
	"context"
	"fmt"
	"sync"

	"github.com/cloudflare/cloudflare-go/v7"
	"github.com/cloudflare/cloudflare-go/v7/cache"
	"github.com/cloudflare/cloudflare-go/v7/option"
	"github.com/zenta-dev/zever/core/cdn"
)

type cloudflareAdapter struct {
	client *cloudflare.Client
	zoneID string
	mu     sync.Mutex
	closed bool
}

var _ cdn.CDN = (*cloudflareAdapter)(nil)

func New(opts cdn.Options) (cdn.CDN, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "https://api.cloudflare.com/client/v4"
	}

	client := cloudflare.NewClient(
		option.WithAPIToken(opts.APIToken),
		option.WithBaseURL(baseURL),
	)

	return &cloudflareAdapter{
		client: client,
		zoneID: opts.ZoneID,
	}, nil
}

func (d *cloudflareAdapter) Purge(ctx context.Context, req cdn.PurgeRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.closed {
		return fmt.Errorf("cloudflare: %w", cdn.ErrClosed)
	}

	if len(req.URLs) == 0 && len(req.Tags) == 0 && !req.All {
		return fmt.Errorf("cdn: empty purge request")
	}

	var body cache.CachePurgeParamsBodyUnion
	switch {
	case req.All:
		body = cache.CachePurgeParamsBodyCachePurgeEverything{
			PurgeEverything: cloudflare.F(true),
		}
	case len(req.Tags) > 0:
		body = cache.CachePurgeParamsBodyCachePurgeFlexPurgeByTags{
			Tags: cloudflare.F(req.Tags),
		}
	default:
		body = cache.CachePurgeParamsBodyCachePurgeSingleFile{
			Files: cloudflare.F(req.URLs),
		}
	}

	_, err := d.client.Cache.Purge(ctx, cache.CachePurgeParams{
		ZoneID: cloudflare.F(d.zoneID),
		Body:   body,
	})
	if err != nil {
		return fmt.Errorf("cloudflare: purge: %w", err)
	}
	return nil
}

func (d *cloudflareAdapter) Close(_ context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	return nil
}

func (d *cloudflareAdapter) Name() string {
	return "cloudflare"
}
