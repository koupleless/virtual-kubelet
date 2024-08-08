package tracker

import (
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/sirupsen/logrus"
	"os"
	"time"
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

	Eventually(string, string, string, map[string]string, model.ErrorCode, func() bool, time.Duration, time.Duration, func(), func())

	ErrorReport(string, string, string, string, map[string]string, model.ErrorCode)
}

func SetTracker(t Tracker) {
	if t == nil {
		logrus.Info("custom tracker is nil")
		return
	}
	T = t
	T.Init()
}

func GetTracker() Tracker {
	return T
}
