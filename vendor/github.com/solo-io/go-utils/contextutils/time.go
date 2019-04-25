package contextutils

import (
	"context"
	"time"
)

func Sleep(ctx context.Context, amount time.Duration) error {
	select {
	case <-time.After(amount):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
