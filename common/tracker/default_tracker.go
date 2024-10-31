package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// Constants for default configuration path and event types
const (
	DefaultConfigPath = "config/default_tracker_config.yaml"
	EventSuccess      = "success"
	EventFail         = "fail"
)

// DefaultTracker is a struct that implements the Tracker interface
var _ Tracker = &DefaultTracker{}

// defaultTrackerConfig is a struct to hold configuration for the DefaultTracker
type defaultTrackerConfig struct {
	LogDir      string   `yaml:"logDir"`      // Directory path for log files
	ReportLevel string   `yaml:"reportLevel"` // Level of reporting, can be "debug" or "error"
	ReportLinks []string `yaml:"reportLinks"` // List of URLs to report events to
}

// isDebug checks if the report level is set to "debug"
func (c defaultTrackerConfig) isDebug() bool {
	return c.ReportLevel == "debug"
}

// logData is a struct to hold data for logging and reporting events
type logData struct {
	TraceID  string            `json:"traceID"`            // Unique identifier for the event
	Scene    string            `json:"scene"`              // Scene or context of the event
	Event    string            `json:"event"`              // Event type, can be "success" or "fail"
	TimeUsed int64             `json:"timeUsed,omitempty"` // Time taken for the event in milliseconds
	Result   string            `json:"result"`             // Result of the event, can be "success" or "fail"
	Message  string            `json:"message"`            // Message or description of the event
	Code     model.ErrorCode   `json:"code"`               // Error code for the event
	Labels   map[string]string `json:"labels"`             // Additional labels or metadata for the event
}

// formatText formats the logData into a string suitable for text logging
func (d logData) formatText() string {
	labelBytes, _ := json.Marshal(d.Labels)
	return fmt.Sprintf("[%s], [%s], [%s], [%s], [%d], [%s], [%s], [%s], [%s]\n",
		time.Now().Format("2006-01-02 15:04:05.000"),
		d.TraceID,
		d.Scene,
		d.Event,
		d.TimeUsed,
		d.Result,
		d.Code,
		d.Message,
		string(labelBytes),
	)
}

// formatJson formats the logData into a JSON string suitable for reporting
func (d logData) formatJson() string {
	jsonBytes, _ := json.Marshal(d)
	return string(jsonBytes)
}

// DefaultTracker is a struct that implements the Tracker interface
type DefaultTracker struct {
	config    defaultTrackerConfig // Configuration for the tracker
	logWriter io.Writer            // Writer for logging events
}

// Init initializes the DefaultTracker with configuration and sets up the log writer
func (t *DefaultTracker) Init() {
	configPath := utils.GetEnv("TRACKER_CONFIG_PATH", DefaultConfigPath)
	configContent, err := os.ReadFile(configPath)
	config := defaultTrackerConfig{
		ReportLevel: "error",
	}
	if err != nil {
		logrus.Error(err)
		t.logWriter = os.Stdout
	} else {
		err = yaml.Unmarshal(configContent, &config)
		if err != nil {
			panic(err)
		}
		t.config = config
		logFile, err := os.OpenFile(path.Join(config.LogDir, "events.log"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Println(err.Error())
			t.logWriter = os.Stdout
		} else {
			t.logWriter = logFile
		}
	}
}

// ErrorReport reports an error event to the configured links and logs it
func (t *DefaultTracker) ErrorReport(traceID, scene, event, message string, labels map[string]string, code model.ErrorCode) {
	// send error report to links in config
	data := logData{
		TraceID: traceID,
		Scene:   scene,
		Event:   event,
		Result:  EventFail,
		Message: message,
		Code:    code,
		Labels:  labels,
	}
	go t.reportEvent(data)
	t.recordEvent(data)
}

// FuncTrack tracks the execution of a function and reports the result
func (t *DefaultTracker) FuncTrack(traceID, scene, event string, labels map[string]string, f func() (error, model.ErrorCode)) error {
	startTime := time.Now()
	err, code := f()
	data := logData{
		TraceID:  traceID,
		Scene:    scene,
		Event:    event,
		Result:   EventSuccess,
		Code:     code,
		TimeUsed: time.Now().Sub(startTime).Milliseconds(),
		Labels:   labels,
	}
	if err != nil {
		data.Result = EventFail
		data.Message = err.Error()
		go t.reportEvent(data)
	} else if t.config.isDebug() {
		go t.reportEvent(data)
	}
	t.recordEvent(data)
	return err
}

// Eventually tracks an event that may timeout and reports the result
func (t *DefaultTracker) Eventually(traceID, scene, event string, labels map[string]string, timeoutErrorCode model.ErrorCode, checkFunc func() bool, timeout time.Duration, interval time.Duration, checkPassed func(), timeoutCall func()) {
	startTime := time.Now()
	data := logData{
		TraceID: traceID,
		Scene:   scene,
		Event:   event,
		Result:  EventSuccess,
		Code:    model.CodeSuccess,
		Labels:  labels,
	}
	checkTimeout := false
	utils.CheckAndFinallyCall(context.Background(), checkFunc, timeout, interval, func() {
		checkPassed()
	}, func() {
		timeoutCall()
		data.Code = timeoutErrorCode
		checkTimeout = true
	})
	data.TimeUsed = time.Now().Sub(startTime).Milliseconds()
	if checkTimeout {
		data.Result = EventFail
		data.Message = "event timeout"
		data.Code = model.CodeSuccess
		go t.reportEvent(data)
	} else if t.config.isDebug() {
		go t.reportEvent(data)
	}
	t.recordEvent(data)
}

// recordEvent records the event to the local log file
func (t *DefaultTracker) recordEvent(data logData) {
	t.logWriter.Write([]byte(data.formatText()))
}

// reportEvent reports the event to the configured links
func (t *DefaultTracker) reportEvent(data logData) {
	for _, link := range t.config.ReportLinks {
		_, err := http.Post(link, "application/json", bytes.NewBufferString(data.formatJson()))
		if err != nil {
			logrus.WithError(err).Error("failed to report event")
		}
	}
}
