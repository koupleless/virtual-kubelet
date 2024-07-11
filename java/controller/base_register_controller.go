package controller

import (
	"context"
	"encoding/json"
	"errors"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/mqtt"
	"github.com/koupleless/virtual-kubelet/java/common"
	"github.com/koupleless/virtual-kubelet/java/model"
	"github.com/koupleless/virtual-kubelet/java/pod/node"
	"github.com/sirupsen/logrus"
	"time"
)

type BaseRegisterController struct {
	config *model.BuildBaseRegisterControllerConfig

	mqttClient *mqtt.Client
	done       chan struct{}
	ready      chan struct{}

	err error

	localStore *RuntimeInfoStore
}

func NewBaseRegisterController(config *model.BuildBaseRegisterControllerConfig) (*BaseRegisterController, error) {
	return &BaseRegisterController{
		config:     config,
		done:       make(chan struct{}),
		ready:      make(chan struct{}),
		localStore: NewRuntimeInfoStore(),
	}, nil
}

func (brc *BaseRegisterController) Run(ctx context.Context) {
	mqttClient, err := mqtt.NewMqttClient(brc.config.MqttConfig)
	if err != nil {
		brc.err = err
		close(brc.done)
		return
	}
	if mqttClient == nil {
		brc.err = errors.New("mqtt client is nil")
		close(brc.done)
		return
	}
	brc.mqttClient = mqttClient

	brc.mqttClient.Sub(BaseHeartBeatTopic, 1, brc.heartBeatMsgCallback)
	brc.mqttClient.Sub(BaseHealthTopic, 1, brc.healthMsgCallback)
	brc.mqttClient.Sub(BaseBizTopic, 1, brc.bizMsgCallback)

	go common.TimedTaskWithInterval(ctx, time.Second*2, brc.checkAndDeleteOfflineBase)
}

func (brc *BaseRegisterController) checkAndDeleteOfflineBase(_ context.Context) {
	offlineDevices := brc.localStore.GetOfflineDevices(1000 * 10)
	for _, deviceID := range offlineDevices {
		kouplelessNode := brc.localStore.GetKouplelessNode(deviceID)
		if kouplelessNode == nil {
			continue
		}
		close(kouplelessNode.BaseBizExitChan)
		brc.localStore.DeleteKouplelessNode(deviceID)
	}
}

func (brc *BaseRegisterController) Done() chan struct{} {
	return brc.done
}

func (brc *BaseRegisterController) Err() error {
	return brc.err
}

func (brc *BaseRegisterController) startVirtualKubelet(deviceID string, initData HeartBeatData) {
	// first apply for local lock
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), "deviceID", deviceID))
	defer cancel()
	if initData.NetworkInfo.LocalIP == "" {
		initData.NetworkInfo.LocalIP = "127.0.0.1"
	}

	// TODO apply for lock in future, to support sharding, after getting lock, create node
	kn, err := node.NewKouplelessNode(&model.BuildKouplelessNodeConfig{
		KubeConfigPath: brc.config.KubeConfigPath,
		MqttClient:     brc.mqttClient,
		NodeID:         deviceID,
		NodeIP:         initData.NetworkInfo.LocalIP,
		TechStack:      "java",
		BizName:        initData.MasterBizInfo.BizName,
		BizVersion:     initData.MasterBizInfo.BizVersion,
	})
	if err != nil {
		logrus.Errorf("Error creating Koleless node: %v", err)
		return
	}

	err = brc.localStore.PutKouplelessNodeNX(deviceID, kn)
	if err != nil {
		// already exist, return
		return
	}

	defer func() {
		// delete from local storage
		brc.localStore.DeleteKouplelessNode(deviceID)
	}()

	go kn.Run(ctx)
	if err = kn.WaitReady(ctx, time.Minute); err != nil {
		logrus.Errorf("Error waiting for Koleless node to become ready: %v", err)
		return
	}
	logrus.Infof("koupleless node running: %s", deviceID)

	// record first msg arrived time
	brc.localStore.DeviceMsgArrived(deviceID)

	select {
	case <-kn.Done():
		logrus.Infof("koupleless node exit: %s", deviceID)
	}
}

func (brc *BaseRegisterController) heartBeatMsgCallback(_ paho.Client, msg paho.Message) {
	defer msg.Ack()
	deviceID := getDeviceIDFromTopic(msg.Topic())
	if deviceID == "" {
		return
	}
	// check local storage
	vNode := brc.localStore.GetKouplelessNode(deviceID)
	if vNode == nil {
		// not started
		var heartBeatMsg ArkMqttMsg[HeartBeatData]
		err := json.Unmarshal(msg.Payload(), &heartBeatMsg)
		if err != nil {
			logrus.Errorf("Error unmarshalling heart beat data: %v", err)
			return
		}
		if expired(heartBeatMsg.PublishTimestamp, 1000*10) {
			return
		}
		go brc.startVirtualKubelet(deviceID, heartBeatMsg.Data)
	} else {
		// only started device set latest msg time
		brc.localStore.DeviceMsgArrived(deviceID)
	}
}

func (brc *BaseRegisterController) healthMsgCallback(_ paho.Client, msg paho.Message) {
	defer msg.Ack()
	deviceID := getDeviceIDFromTopic(msg.Topic())
	if deviceID == "" {
		return
	}
	var data ArkMqttMsg[ark.HealthResponse]
	err := json.Unmarshal(msg.Payload(), &data)
	if err != nil {
		logrus.Errorf("Error unmarshalling health response: %v", err)
		return
	}
	if expired(data.PublishTimestamp, 1000*10) {
		return
	}
	if data.Data.Code != "SUCCESS" {
		return
	}
	kouplelessNode := brc.localStore.GetKouplelessNode(deviceID)
	if kouplelessNode == nil {
		return
	}
	brc.localStore.DeviceMsgArrived(deviceID)

	kouplelessNode.BaseHealthInfoChan <- data.Data.Data.HealthData
}

func (brc *BaseRegisterController) bizMsgCallback(_ paho.Client, msg paho.Message) {
	defer msg.Ack()
	deviceID := getDeviceIDFromTopic(msg.Topic())
	if deviceID == "" {
		return
	}
	var data ArkMqttMsg[ark.QueryAllArkBizResponse]
	err := json.Unmarshal(msg.Payload(), &data)
	if err != nil {
		logrus.Errorf("Error unmarshalling biz response: %v", err)
		return
	}
	if expired(data.PublishTimestamp, 1000*10) {
		return
	}
	if data.Data.Code != "SUCCESS" {
		return
	}
	kouplelessNode := brc.localStore.GetKouplelessNode(deviceID)
	if kouplelessNode == nil {
		return
	}
	brc.localStore.DeviceMsgArrived(deviceID)
	kouplelessNode.BaseBizInfoChan <- data.Data.Data
}
