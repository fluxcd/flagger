package contextutils

import (
	"context"
	"errors"
	"math/rand"
	"time"
)

const (
	// baseDelay is the amount of time to wait before retrying after the first
	// failure.
	baseDelay = 1.0 * time.Second
	// factor is applied to the backoff after each retry.
	factor = 1.6
	// jitter provides a range to randomize backoff delays.
	jitter = 0.2
)

type ExponentioalBackoff struct {
	MaxRetries  uint
	MaxDuration *time.Duration
	MaxDelay    *time.Duration
}

type Backoff interface {
	Backoff(ctx context.Context, f func(ctx context.Context) error) error
}

func NewExponentioalBackoff(eb ExponentioalBackoff) Backoff {
	if eb.MaxDelay == nil {
		tmp := 15 * time.Minute
		eb.MaxDelay = &tmp
	}

	return &exponentioalBackoff{
		MaxRetries:  eb.MaxRetries,
		MaxDuration: eb.MaxDuration,
		MaxDelay:    *eb.MaxDelay,
	}
}

type exponentioalBackoff struct {
	MaxRetries  uint
	MaxDuration *time.Duration
	MaxDelay    time.Duration

	start *time.Duration
}

func (e *exponentioalBackoff) Backoff(ctx context.Context, f func(ctx context.Context) error) error {
	retries := uint(0)
	if e.MaxDuration != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *e.MaxDuration)
		defer cancel()
	}

	for {

		err := f(ctx)

		if ctx.Err() != nil {
			return ctx.Err()
		}

		if err == nil {
			return nil
		}
		LoggerFrom(ctx).Debugf("error in exponential backoff: %v", err)
		if e.MaxRetries != 0 && retries > e.MaxRetries {
			return errors.New("max retries exceeded")
		}
		timetosleep := e.calcTimeToSleep(retries)
		retries++

		err = Sleep(ctx, timetosleep)
		if err != nil {
			return nil
		}
	}

}

// inspired by: https://github.com/grpc/grpc-go/blob/ce4f3c8a89229d9db3e0c30d28a9f905435ad365/internal/backoff/backoff.go#L59
func (e *exponentioalBackoff) calcTimeToSleep(retries uint) time.Duration {
	if retries == 0 {
		return baseDelay
	}
	backoff, max := float64(baseDelay), float64(e.MaxDelay)
	for backoff < max && retries > 0 {
		backoff *= factor
		retries--
	}
	if backoff > max {
		backoff = max
	}
	// Randomize backoff delays so that if a cluster of requests start at
	// the same time, they won't operate in lockstep.
	backoff *= 1 + jitter*(rand.Float64()*2-1)
	if backoff < 0 {
		return 0
	}
	return time.Duration(backoff)
}
