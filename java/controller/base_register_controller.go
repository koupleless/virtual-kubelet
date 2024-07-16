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
	"github.com/virtual-kubelet/virtual-kubelet/log"
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
	brc.config.MqttConfig.OnConnectHandler = func(client paho.Client) {
		log.G(ctx).Info("Connected")
		reader := client.OptionsReader()
		log.G(ctx).Info("Connect options: ", reader.ClientID(), reader.Username(), reader.Password())
		client.Subscribe(BaseHeartBeatTopic, 1, brc.heartBeatMsgCallback)
		client.Subscribe(BaseHealthTopic, 1, brc.healthMsgCallback)
		client.Subscribe(BaseBizTopic, 1, brc.bizMsgCallback)
	}
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

	go common.TimedTaskWithInterval(ctx, time.Second*2, brc.checkAndDeleteOfflineBase)
}

func (brc *BaseRegisterController) checkAndDeleteOfflineBase(_ context.Context) {
	offlineBase := brc.localStore.GetOfflineBases(1000 * 10)
	for _, baseID := range offlineBase {
		kouplelessNode := brc.localStore.GetKouplelessNode(baseID)
		if kouplelessNode == nil {
			continue
		}
		close(kouplelessNode.BaseBizExitChan)
		brc.localStore.DeleteKouplelessNode(baseID)
	}
}

func (brc *BaseRegisterController) Done() chan struct{} {
	return brc.done
}

func (brc *BaseRegisterController) Err() error {
	return brc.err
}

func (brc *BaseRegisterController) startVirtualKubelet(baseID string, initData HeartBeatData) {
	// first apply for local lock
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), "baseID", baseID))
	defer cancel()
	if initData.NetworkInfo.LocalIP == "" {
		initData.NetworkInfo.LocalIP = "127.0.0.1"
	}

	// TODO apply for lock in future, to support sharding, after getting lock, create node
	kn, err := node.NewKouplelessNode(&model.BuildKouplelessNodeConfig{
		KubeConfigPath: brc.config.KubeConfigPath,
		MqttClient:     brc.mqttClient,
		NodeID:         baseID,
		NodeIP:         initData.NetworkInfo.LocalIP,
		TechStack:      "java",
		BizName:        initData.MasterBizInfo.BizName,
		BizVersion:     initData.MasterBizInfo.BizVersion,
	})
	if err != nil {
		logrus.Errorf("Error creating Koleless node: %v", err)
		return
	}

	err = brc.localStore.PutKouplelessNodeNX(baseID, kn)
	if err != nil {
		// already exist, return
		return
	}

	defer func() {
		// delete from local storage
		brc.localStore.DeleteKouplelessNode(baseID)
	}()

	go kn.Run(ctx)
	if err = kn.WaitReady(ctx, time.Minute); err != nil {
		logrus.Errorf("Error waiting for Koleless node to become ready: %v", err)
		return
	}
	logrus.Infof("koupleless node running: %s", baseID)

	// record first msg arrived time
	brc.localStore.BaseMsgArrived(baseID)

	select {
	case <-kn.Done():
		logrus.Infof("koupleless node exit: %s", baseID)
	case <-ctx.Done():
	}
}

func (brc *BaseRegisterController) heartBeatMsgCallback(_ paho.Client, msg paho.Message) {
	defer msg.Ack()
	baseID := getBaseIDFromTopic(msg.Topic())
	if baseID == "" {
		return
	}
	var heartBeatMsg ArkMqttMsg[HeartBeatData]
	err := json.Unmarshal(msg.Payload(), &heartBeatMsg)
	if err != nil {
		logrus.Errorf("Error unmarshalling heart beat data: %v", err)
		return
	}
	if expired(heartBeatMsg.PublishTimestamp, 1000*10) {
		return
	}
	// check local storage
	vNode := brc.localStore.GetKouplelessNode(baseID)
	if vNode == nil {
		// not started
		go brc.startVirtualKubelet(baseID, heartBeatMsg.Data)
	} else {
		// only started base set latest msg time
		brc.localStore.BaseMsgArrived(baseID)
	}
}

func (brc *BaseRegisterController) healthMsgCallback(_ paho.Client, msg paho.Message) {
	defer msg.Ack()
	baseID := getBaseIDFromTopic(msg.Topic())
	if baseID == "" {
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
	kouplelessNode := brc.localStore.GetKouplelessNode(baseID)
	if kouplelessNode == nil {
		return
	}
	brc.localStore.BaseMsgArrived(baseID)

	select {
	case kouplelessNode.BaseHealthInfoChan <- data.Data.Data.HealthData:
	default:
	}
}

func (brc *BaseRegisterController) bizMsgCallback(_ paho.Client, msg paho.Message) {
	defer msg.Ack()
	baseID := getBaseIDFromTopic(msg.Topic())
	if baseID == "" {
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
	kouplelessNode := brc.localStore.GetKouplelessNode(baseID)
	if kouplelessNode == nil {
		return
	}
	brc.localStore.BaseMsgArrived(baseID)
	select {
	case kouplelessNode.BaseBizInfoChan <- data.Data.Data:
	default:
	}
}
