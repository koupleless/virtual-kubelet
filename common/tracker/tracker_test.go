package tracker

import (
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
	"time"
)

var _ Tracker = &mockTracker{}

type mockTracker struct {
}

func (m mockTracker) FuncTrack(s string, s2 string, s3 string, m2 map[string]string, f func() (error, model.ErrorCode)) error {
	panic("implement me")
}

func (m mockTracker) Eventually(s string, s2 string, s3 string, m2 map[string]string, code model.ErrorCode, f func() (bool, error), duration time.Duration, duration2 time.Duration, f2 func(), f3 func()) {
	panic("implement me")
}

func (m mockTracker) ErrorReport(s string, s2 string, s3 string, s4 string, m2 map[string]string, code model.ErrorCode) {
	panic("implement me")
}

func (m mockTracker) Init() {
	return
}

func TestSetTracker_Nil(t *testing.T) {
	SetTracker(nil)
	assert.NotNil(t, G())
}

func TestSetTracker_NoNil(t *testing.T) {
	SetTracker(&mockTracker{})
	name := reflect.TypeOf(G()).String()
	assert.True(t, name == "*tracker.mockTracker")
}
