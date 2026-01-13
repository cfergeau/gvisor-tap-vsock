package utils

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
)

const maxRetries = 60
const initialBackoff = 100 * time.Millisecond

type NonRetriableError struct {
	Err error
}

func NewNonRetriableError(err error) error {
	return &NonRetriableError{Err: err}
}

func (err *NonRetriableError) Error() string {
	return err.Err.Error()
}

func (err *NonRetriableError) Unwrap() error {
	return err.Err
}

func Retry[T comparable](ctx context.Context, retryFunc func() (T, error), retryMsg string) (T, error) {
	return RetryWithInitialBackoff(ctx, retryFunc, retryMsg, initialBackoff)
}

func RetryWithInitialBackoff[T comparable](ctx context.Context, retryFunc func() (T, error), retryMsg string, initialBackoff time.Duration) (T, error) {
	var (
		returnVal T
		err       error
	)

	backoff := initialBackoff

loop:
	for range maxRetries {
		select {
		case <-ctx.Done():
			break loop
		default:
			// proceed
		}

		returnVal, err = retryFunc()
		if err == nil {
			return returnVal, nil
		}
		var nonRetriableErr *NonRetriableError
		if errors.As(err, &nonRetriableErr) {
			return returnVal, nonRetriableErr.Err
		}
		logrus.Debugf("%s (%s)", retryMsg, backoff)
		Sleep(ctx, backoff)
		backoff = backOff(backoff)
	}
	return returnVal, fmt.Errorf("timeout: %w", err)
}

func backOff(delay time.Duration) time.Duration {
	if delay == 0 {
		delay = 5 * time.Millisecond
	} else {
		delay *= 2
	}
	if delay > time.Second {
		delay = time.Second
	}
	return delay
}

func Sleep(ctx context.Context, wait time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(wait):
		return true
	}
}
