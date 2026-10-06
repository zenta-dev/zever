package resilience

import "context"

// Do runs fn under g and returns its typed result, so callers avoid the
// intermediate error-only closure that Guard.Execute requires. A non-nil
// error from fn is returned unwrapped after passing through g's policies.
func Do[T any](ctx context.Context, g Guard, fn func(context.Context) (T, error)) (T, error) {
	var out T

	err := g.Execute(ctx, func(ctx context.Context) error {
		v, err := fn(ctx)
		if err != nil {
			return err
		}

		out = v

		return nil
	})
	if err != nil {
		var zero T
		return zero, err
	}

	return out, nil
}
