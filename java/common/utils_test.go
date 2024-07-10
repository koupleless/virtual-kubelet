package common

import (
	"context"
	"github.com/koupleless/virtual-kubelet/java/model"
	"gotest.tools/assert"
	"testing"
	"time"
)

func TestTimedTaskWithInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testList := make([]int, 0)
	go TimedTaskWithInterval(ctx, time.Second, func(_ context.Context) {
		testList = append(testList, 0)
	})
	time.Sleep(2500 * time.Millisecond)
	assert.Assert(t, len(testList) == 3)
}

func TestTimedTaskWithInterval_cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	testList := make([]int, 0)
	go func() {
		time.Sleep(time.Millisecond * 1010)
		cancel()
	}()
	TimedTaskWithInterval(ctx, time.Second, func(_ context.Context) {
		testList = append(testList, 0)
	})
	assert.Assert(t, len(testList) == 2)
}

func TestConvertByteNumToResourceQuantity(t *testing.T) {
	quantity := ConvertByteNumToResourceQuantity(-1)
	assert.Assert(t, quantity.IsZero())
	quantity = ConvertByteNumToResourceQuantity(1)
	assert.Assert(t, quantity.IsZero())
	quantity = ConvertByteNumToResourceQuantity(1025)
	assert.Assert(t, quantity.String() == "1Ki")
}

func TestFormatArkletCommandTopic(t *testing.T) {
	topic := FormatArkletCommandTopic("test", model.CommandHealth)
	assert.Assert(t, topic == "koupleless/test/health")
}
