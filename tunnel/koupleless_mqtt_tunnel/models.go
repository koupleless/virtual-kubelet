package koupleless_mqtt_tunnel

import "github.com/koupleless/arkctl/v1/service/ark"

// ArkMqttMsg is the response of mqtt message payload.
type ArkMqttMsg[T any] struct {
	PublishTimestamp int64 `json:"publishTimestamp"`
	Data             T     `json:"data"`
}

// HeartBeatData is the data of base heart beat.
type HeartBeatData struct {
	DeviceID      string            `json:"deviceID"`
	MasterBizInfo ark.MasterBizInfo `json:"masterBizInfo"`
	NetworkInfo   NetworkInfo       `json:"networkInfo"`
}

type NetworkInfo struct {
	LocalIP       string `json:"localIP"`
	LocalHostName string `json:"localHostName"`
	ArkletPort    int    `json:"arkletPort"`
}

type BizOperationResponse struct {
	Command    string              `json:"command"`
	BizName    string              `json:"bizName"`
	BizVersion string              `json:"bizVersion"`
	Response   ark.ArkResponseBase `json:"response"`
}
