package utils

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/resource"
	"time"
)

func TimedTaskWithInterval(ctx context.Context, interval time.Duration, task func(context.Context)) {
	task(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			task(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func ConvertByteNumToResourceQuantity(byteNum int64) resource.Quantity {
	resourceStr := ""
	byteNum /= 1024
	if byteNum <= 0 {
		byteNum = 0
	}
	resourceStr = fmt.Sprintf("%dKi", byteNum)
	return resource.MustParse(resourceStr)
}
