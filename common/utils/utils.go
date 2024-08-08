package utils

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/resource"
	"os"
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

func CheckAndFinallyCall(checkFunc func() bool, timeout, interval time.Duration, finally, timeoutCall func()) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	checkTicker := time.NewTicker(interval)
	for range checkTicker.C {
		select {
		case <-ctx.Done():
			// TODO timeout log
			logrus.Info("Check and Finally call timeout")
			timeoutCall()
			return
		default:
		}
		if checkFunc() {
			finally()
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

func GetEnv(key, defaultValue string) string {
	value, found := os.LookupEnv(key)
	if found {
		return value
	}
	return defaultValue
}
