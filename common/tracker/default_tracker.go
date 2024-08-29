package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/sirupsen/logrus"
	"io"
	"k8s.io/apimachinery/pkg/util/yaml"
	"log"
	"net/http"
	"os"
	"path"
	"time"
)

const (
	DefaultConfigPath = "config/default_tracker_config.yaml"
	EventSuccess      = "success"
	EventFail         = "fail"
)

var _ Tracker = &DefaultTracker{}

type defaultTrackerConfig struct {
	LogDir      string   `yaml:"logDir"`
	ReportLevel string   `yaml:"reportLevel"`
	ReportLinks []string `yaml:"reportLinks"`
}

func (c defaultTrackerConfig) isDebug() bool {
	return c.ReportLevel == "debug"
}

type logData struct {
	TraceID  string            `json:"traceID"`
	Scene    string            `json:"scene"`
	Event    string            `json:"event"`
	TimeUsed int64             `json:"timeUsed,omitempty"`
	Result   string            `json:"result"`
	Message  string            `json:"message"`
	Code     model.ErrorCode   `json:"code"`
	Labels   map[string]string `json:"labels"`
}

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

func (d logData) formatJson() string {
	jsonBytes, _ := json.Marshal(d)
	return string(jsonBytes)
}

type DefaultTracker struct {
	config    defaultTrackerConfig
	logWriter io.Writer
}

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

// record event to local log file
func (t *DefaultTracker) recordEvent(data logData) {
	t.logWriter.Write([]byte(data.formatText()))
}

// report event to links
func (t *DefaultTracker) reportEvent(data logData) {
	for _, link := range t.config.ReportLinks {
		_, err := http.Post(link, "application/json", bytes.NewBufferString(data.formatJson()))
		if err != nil {
			logrus.WithError(err).Error("failed to report event")
		}
	}
}
