package controller

import (
	"strings"
	"time"
)

func getDeviceIDFromTopic(topic string) string {
	fileds := strings.Split(topic, "/")
	if len(fileds) < 2 {
		return ""
	}
	if fileds[0] != "koupleless" {
		return ""
	}
	return fileds[1]
}

func expired(publishTimestamp int64, maxLiveMilliSec int64) bool {
	return publishTimestamp+maxLiveMilliSec <= time.Now().UnixMilli()
}
