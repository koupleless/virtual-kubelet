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

// close by deactive message
// or timeout for module.NodeLeaseDurationSeconds, this may caused by base offline or leader changed
func (liveness *Liveness) IsDead() bool {
	return liveness.isClose || time.Since(liveness.LatestHeartBeatTime) >= model.NodeLeaseDurationSeconds*time.Second
}

// close because the base upload the deactive message
func (liveness *Liveness) Close() {
	liveness.isClose = true
	liveness.LatestHeartBeatTime = time.Now()
}

// unReachable check the node is unreachable
func (liveness *Liveness) IsReachable() bool {
	return !liveness.isClose && time.Since(liveness.LatestHeartBeatTime) <= model.NodeLeaseUpdatePeriodSeconds*time.Second
}
