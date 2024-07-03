package common

import (
	"context"
	"fmt"
	"github.com/koupleless/virtual-kubelet/java/model"
	"k8s.io/apimachinery/pkg/api/resource"
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

func ConvertByteNumToResourceQuantity(byteNum int64) resource.Quantity {
	resourceStr := ""
	byteNum /= 1024
	if byteNum <= 0 {
		byteNum = 0
	}
	resourceStr = fmt.Sprintf("%dKi", byteNum)
	return resource.MustParse(resourceStr)
}
