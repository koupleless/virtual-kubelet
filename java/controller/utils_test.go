package controller

import (
	"gotest.tools/assert"
	"testing"
	"time"
)

func TestGetDeviceID_InvalidLen(t *testing.T) {
	id := getDeviceIDFromTopic("test")
	assert.Assert(t, id == "")
}

func TestGetDeviceID_InvalidPrefix(t *testing.T) {
	id := getDeviceIDFromTopic("test/test")
	assert.Assert(t, id == "")
}

func TestGetDeviceID_Valid(t *testing.T) {
	id := getDeviceIDFromTopic("koupleless/test")
	assert.Assert(t, id == "test")
}

func TestExpired(t *testing.T) {
	assert.Assert(t, expired(0, 1000*10))
	assert.Assert(t, !expired(time.Now().UnixMilli(), 1000*10))
}
