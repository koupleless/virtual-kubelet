package common

import (
	"context"
	"time"
)

func TimedTaskWithInterval(taskName string, interval time.Duration, task func(context.Context)) {
	ctx := context.WithValue(context.Background(), "timed task name", taskName)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		task(ctx)
	}
}
