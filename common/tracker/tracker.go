package tracker

import (
	"context"
	"os"
	"time"

	"github.com/koupleless/virtual-kubelet/model"
	"github.com/sirupsen/logrus"
)

var (
	G = GetTracker

	T Tracker = &DefaultTracker{
		logWriter: os.Stdout,
	}
)

type Tracker interface {
	Init()

	FuncTrack(string, string, string, map[string]string, func() (error, model.ErrorCode)) error

	Eventually(string, string, string, map[string]string, model.ErrorCode, func(ctx context.Context) (bool, error), time.Duration, time.Duration, func(), func())

	ErrorReport(string, string, string, string, map[string]string, model.ErrorCode)
}

// Set the custom tracker
func SetTracker(t Tracker) {
	if t == nil {
		logrus.Info("custom tracker is nil")
		return
	}
	T = t
	T.Init()
}

// Get the current tracker
func GetTracker() Tracker {
	return T
}
