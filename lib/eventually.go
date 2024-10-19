package lib

import (
	"fmt"
	"time"
)

func Eventually(condition func() bool, timeout time.Duration, interval time.Duration) error {
	end := time.Now().Add(timeout)

	for time.Now().Before(end) {
		if condition() {
			return nil
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("condition was not met within %s", timeout)
}
