package agent

import (
	"context"
	"sync"

	"github.com/zenta-dev/zever/core/ai"
)

// dispatchAll resolves and runs every call, returning outputs in call order
// regardless of execution order. Fail-closed errors (unknown tool,
// confirmation failure, canceled context) abort the batch; handler and
// argument failures become output strings for model self-correction, exactly
// as in sequential dispatch.
func (l *Loop) dispatchAll(ctx context.Context, calls []ai.ToolCall) ([]string, error) {
	if l.opts.MaxParallel <= 1 || len(calls) < 2 {
		return l.dispatchSequential(ctx, calls)
	}

	return l.dispatchParallel(ctx, calls)
}

// dispatchSequential runs calls one by one in order.
func (l *Loop) dispatchSequential(ctx context.Context, calls []ai.ToolCall) ([]string, error) {
	out := make([]string, 0, len(calls))

	for _, call := range calls {
		res, err := l.dispatch(ctx, call)
		if err != nil {
			return nil, err
		}

		out = append(out, res)
	}

	return out, nil
}

// dispatchParallel runs calls concurrently bounded by MaxParallel, preserving
// result order. The first fail-closed error cancels the batch; in-flight
// handlers observe cancellation through ctx.
func (l *Loop) dispatchParallel(ctx context.Context, calls []ai.ToolCall) ([]string, error) {
	limit := l.opts.MaxParallel
	if limit <= 0 || limit > len(calls) {
		limit = len(calls)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	out := make([]string, len(calls))
	sem := make(chan struct{}, limit)
	errCh := make(chan error, 1)

	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)

		go func() {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			res, err := l.dispatch(ctx, call)
			if err != nil {
				select {
				case errCh <- err:
				default:
				}

				cancel()

				return
			}

			out[i] = res
		}()
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return nil, err
	default:
		return out, nil
	}
}
