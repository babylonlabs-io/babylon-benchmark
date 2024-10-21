package lib

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func Eventually(ctx context.Context, condition func() bool, timeout time.Duration, interval time.Duration, msgAndArgs ...interface{}) error {
	deadline := time.Now().Add(timeout)
	for now := time.Now(); now.Before(deadline); now = time.Now() {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context canceled")
		default:
			if condition() {
				return nil
			}
			time.Sleep(interval)
		}
	}

	msg := "condition not met within timeout"
	if len(msgAndArgs) > 0 {
		msg = fmt.Sprintf(msg, msgAndArgs...)
	}

	return errors.New(msg)
}
