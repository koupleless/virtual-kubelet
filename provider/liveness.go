package provider

import (
	"github.com/koupleless/virtual-kubelet/model"
	"time"
)

// Liveness sync and check the status in provider
type Liveness struct {
	// for fast close, we need to close manually, or we need to wait to timeout
	isClose             bool
	LatestHeartBeatTime time.Time
}

func (liveness *Liveness) UpdateHeartBeatTime() {
	liveness.isClose = false
	liveness.LatestHeartBeatTime = time.Now()
}

func (liveness *Liveness) IsAlive() bool {
	return !liveness.isClose && time.Since(liveness.LatestHeartBeatTime) < model.NodeLeaseDurationSeconds*time.Second
}

func (liveness *Liveness) Close() {
	liveness.isClose = true
}
