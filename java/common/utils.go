package common

import (
	"context"
	"github.com/koupleless/virtual-kubelet/java/model"
	"time"
)

func TimedTaskWithInterval(taskName string, interval time.Duration, task func(context.Context)) {
	ctx := context.WithValue(context.Background(), model.TimedTaskNameKey, taskName)
	task(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		task(ctx)
	}
}
