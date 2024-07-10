package controller

import (
	"github.com/koupleless/arkctl/v1/service/ark"
	"k8s.io/client-go/kubernetes"
)

const (
	BaseHeartBeatTopic = "koupleless/+/base/heart"
	BaseHealthTopic    = "koupleless/+/base/health"
	BaseBizTopic       = "koupleless/+/base/biz"
)

var clientSet *kubernetes.Clientset

// HeartBeatData is the data of base heart beat.
type HeartBeatData struct {
	MasterBizInfo ark.MasterBizInfo `json:"masterBizInfo"`
	NetworkInfo   struct {
		LocalIP       string `json:"localIP"`
		LocalHostName string `json:"localHostName"`
	} `json:"networkInfo"`
}

// ArkMqttMsg is the response of mqtt message payload.
type ArkMqttMsg[T any] struct {
	PublishTimestamp int64 `json:"publishTimestamp"`
	Data             T     `json:"data"`
}
