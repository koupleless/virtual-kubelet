package mqtt_tunnel

import (
	"github.com/koupleless/virtual-kubelet/model"
	"gotest.tools/assert"
	"testing"
	"time"
)

func TestGetBaseID_InvalidLen(t *testing.T) {
	id := getBaseIDFromTopic("test")
	assert.Assert(t, id == "")
}

func TestGetBaseID_Valid(t *testing.T) {
	id := getBaseIDFromTopic("koupleless/test")
	assert.Assert(t, id == "test")
}

func TestExpired(t *testing.T) {
	assert.Assert(t, expired(0, 1000*10))
	assert.Assert(t, !expired(time.Now().UnixMilli(), 1000*10))
}

func TestFormatArkletCommandTopic(t *testing.T) {
	topic := formatArkletCommandTopic("test", "test", model.CommandHealth)
	assert.Assert(t, topic == "koupleless_test/test/health")
}
