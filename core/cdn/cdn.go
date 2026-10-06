package cdn

import (
	"context"
	"fmt"

	"github.com/zenta-dev/zever/shared/registry"
)

type CDN interface {
	Purge(ctx context.Context, req PurgeRequest) error
	Close(ctx context.Context) error
	Name() string
}

type PurgeRequest struct {
	URLs []string
	Tags []string
	All  bool
}

type Factory func(opts Options) (CDN, error)

var factories = registry.New[Adapter, Factory](
	ErrNilFactory,
	func(adapter Adapter) error { return DuplicateError{Adapter: adapter} },
	func(adapter Adapter) error { return UnknownAdapterError{Adapter: adapter} },
)

func Register(adapter Adapter, factory Factory) error {
	if factory == nil {
		return fmt.Errorf("%w for adapter %s", ErrNilFactory, adapter)
	}
	return factories.Register(adapter, factory)
}

func Open(adapter Adapter, opts Options) (CDN, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	factory, err := factories.Lookup(adapter)
	if err != nil {
		return nil, err
	}
	c, err := factory(opts)
	if err != nil {
		return nil, fmt.Errorf("cdn: open %s: %w", adapter, err)
	}
	return c, nil
}
