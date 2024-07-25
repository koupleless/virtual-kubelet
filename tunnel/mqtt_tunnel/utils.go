package mqtt_tunnel

import (
	"fmt"
	"strings"
	"time"
)

func getBaseIDFromTopic(topic string) string {
	fileds := strings.Split(topic, "/")
	if len(fileds) < 2 {
		return ""
	}
	return fileds[1]
}

func expired(publishTimestamp int64, maxLiveMilliSec int64) bool {
	return publishTimestamp+maxLiveMilliSec <= time.Now().UnixMilli()
}

func formatArkletCommandTopic(baseID, command string) string {
	return fmt.Sprintf("koupleless/%s/%s", baseID, command)
}
