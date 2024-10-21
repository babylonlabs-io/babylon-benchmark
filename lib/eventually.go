package lib

import (
	"fmt"
	"time"
)

//func Eventually(condition func() bool, timeout time.Duration, interval time.Duration) error {
//	end := time.Now().Add(timeout)
//
//	for time.Now().Before(end) {
//		if condition() {
//			return nil
//		}
//		time.Sleep(interval)
//	}
//	return fmt.Errorf("condition was not met within %s", timeout)
//}

func Eventually(condition func() bool, timeout time.Duration, interval time.Duration, msgAndArgs ...interface{}) error {
	deadline := time.Now().Add(timeout)
	for now := time.Now(); now.Before(deadline); now = time.Now() {
		if condition() {
			return nil
		}
		time.Sleep(interval)
	}

	msg := "condition not met within timeout"
	if len(msgAndArgs) > 0 {
		msg = fmt.Sprintf(msg, msgAndArgs...)
	}
	return fmt.Errorf(msg)
}
