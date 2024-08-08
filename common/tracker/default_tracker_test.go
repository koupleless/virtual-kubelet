package tracker

import (
	"errors"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/stretchr/testify/assert"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

var _ http.Handler = &logHandler{}

type logHandler struct {
	Message    string
	msgArrived chan struct{}
}

func (h *logHandler) waitForMsg() {
	<-h.msgArrived
}

var handler = logHandler{}

func init() {
	http.Handle("/log", &handler)
}

func initMockServer() {
	handler.msgArrived = make(chan struct{})
	go http.ListenAndServe(":8080", nil)
}

func (l *logHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	defer request.Body.Close()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return
	}
	l.Message = string(body)
	select {
	case <-l.msgArrived:
	default:
		close(l.msgArrived)
	}
	return
}

func TestDefaultTracker_InitWithoutConfigFile(t *testing.T) {
	os.Setenv("TRACKER_CONFIG_PATH", "none")
	defer os.Unsetenv("TRACKER_CONFIG_PATH")
	tracker := DefaultTracker{}
	tracker.Init()
}

func TestDefaultTracker_InitWithConfigFile(t *testing.T) {
	os.Setenv("TRACKER_CONFIG_PATH", "../../config/default_tracker_config.yaml")
	defer os.Unsetenv("TRACKER_CONFIG_PATH")
	tracker := DefaultTracker{}
	tracker.Init()
}

func TestDefaultTracker_ErrorReportNoServer(t *testing.T) {
	os.Setenv("TRACKER_CONFIG_PATH", "../../config/default_tracker_config.yaml")
	defer os.Unsetenv("TRACKER_CONFIG_PATH")
	tracker := DefaultTracker{}
	tracker.Init()
	tracker.ErrorReport("test_trace", "test_scene", "test_event", "test_message", map[string]string{}, model.CodeSuccess)
}

func TestDefaultTracker_ErrorReport(t *testing.T) {
	os.Setenv("TRACKER_CONFIG_PATH", "../../config/default_tracker_config.yaml")
	defer os.Unsetenv("TRACKER_CONFIG_PATH")
	initMockServer()
	tracker := DefaultTracker{}
	tracker.Init()
	tracker.ErrorReport("test_trace", "test_scene", "test_event", "test_message", map[string]string{}, model.CodeSuccess)
}

func TestDefaultTracker_FuncTrackNoError(t *testing.T) {
	os.Setenv("TRACKER_CONFIG_PATH", "../../config/default_tracker_config.yaml")
	defer os.Unsetenv("TRACKER_CONFIG_PATH")
	initMockServer()
	tracker := DefaultTracker{}
	tracker.Init()

	tracker.FuncTrack("test_trace", "test_scene", "test_event", map[string]string{}, func() (error, model.ErrorCode) {
		return nil, model.CodeSuccess
	})
}

func TestDefaultTracker_FuncTrackError(t *testing.T) {
	os.Setenv("TRACKER_CONFIG_PATH", "../../config/default_tracker_config.yaml")
	defer os.Unsetenv("TRACKER_CONFIG_PATH")
	initMockServer()
	tracker := DefaultTracker{}
	tracker.Init()

	err := tracker.FuncTrack("test_trace", "test_scene", "test_event", map[string]string{}, func() (error, model.ErrorCode) {
		return errors.New("test_error"), model.CodeSuccess
	})
	assert.Error(t, err)
}

func TestDefaultTracker_EventuallyTimeout(t *testing.T) {
	os.Setenv("TRACKER_CONFIG_PATH", "../../config/default_tracker_config.yaml")
	defer os.Unsetenv("TRACKER_CONFIG_PATH")
	initMockServer()
	tracker := DefaultTracker{}
	tracker.Init()

	timeout := false
	checkPass := false
	tracker.Eventually("test_trace", "test_scene", "test_event", map[string]string{}, model.CodeTimeout, func() bool {
		return false
	}, time.Second, time.Millisecond*100, func() {
		checkPass = true
		return
	}, func() {
		timeout = true
		return
	})
	assert.True(t, timeout)
	assert.False(t, checkPass)
}

func TestDefaultTracker_EventuallyNoTimeout(t *testing.T) {
	os.Setenv("TRACKER_CONFIG_PATH", "../../config/default_tracker_config.yaml")
	defer os.Unsetenv("TRACKER_CONFIG_PATH")
	initMockServer()
	tracker := DefaultTracker{}
	tracker.Init()

	timeout := false
	checkPass := false
	tracker.Eventually("test_trace", "test_scene", "test_event", map[string]string{}, model.CodeTimeout, func() bool {
		return true
	}, time.Second, time.Millisecond*100, func() {
		checkPass = true
		return
	}, func() {
		timeout = true
		return
	})
	assert.True(t, checkPass)
	assert.False(t, timeout)
}
