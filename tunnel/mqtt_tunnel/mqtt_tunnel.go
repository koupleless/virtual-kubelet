package mqtt_tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/mqtt"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/sirupsen/logrus"
)

var _ tunnel.Tunnel = &MqttTunnel{}

type MqttTunnel struct {
	mqttClient *mqtt.Client

	baseDiscoveredCallback         func(string, model.HeartBeatData, tunnel.Tunnel)
	healthDataArrivedCallback      func(string, ark.HealthData)
	queryAllBizDataArrivedCallback func(string, []ark.ArkBizInfo)
}

func (m *MqttTunnel) OnBaseStart(ctx context.Context, baseID string) {
	err := m.mqttClient.Sub(fmt.Sprintf(model.BaseHealthTopic, baseID), mqtt.Qos1, m.healthMsgCallback)
	if err != nil {
		log.G(ctx).Error(err.Error())
	}
	err = m.mqttClient.Sub(fmt.Sprintf(model.BaseBizTopic, baseID), mqtt.Qos1, m.bizMsgCallback)
	if err != nil {
		log.G(ctx).Error(err.Error())
	}
}

func (m *MqttTunnel) OnBaseStop(ctx context.Context, baseID string) {
	err := m.mqttClient.UnSub(fmt.Sprintf(model.BaseHealthTopic, baseID))
	if err != nil {
		log.G(ctx).Error(err.Error())
	}
	err = m.mqttClient.UnSub(fmt.Sprintf(model.BaseBizTopic, baseID))
	if err != nil {
		log.G(ctx).Error(err.Error())
	}
}

func (m *MqttTunnel) Name() string {
	return "mqtt_tunnel_provider"
}

func (m *MqttTunnel) Register(ctx context.Context, clientID string, baseDiscoveredCallback tunnel.BaseDiscoveredCallback, healthDataArrivedCallback tunnel.HealthDataArrivedCallback, queryAllBizDataArrivedCallback tunnel.QueryAllBizDataArrivedCallback) (err error) {
	m.baseDiscoveredCallback = baseDiscoveredCallback

	m.healthDataArrivedCallback = healthDataArrivedCallback

	m.queryAllBizDataArrivedCallback = queryAllBizDataArrivedCallback

	c := &MqttConfig{}
	c.init()
	clientID = fmt.Sprintf("%s@@@%s", c.MqttClientPrefix, clientID)
	m.mqttClient, err = mqtt.NewMqttClient(&mqtt.ClientConfig{
		Broker:        c.MqttBroker,
		Port:          c.MqttPort,
		ClientID:      clientID,
		Username:      c.MqttUsername,
		Password:      c.MqttPassword,
		CAPath:        c.MqttCAPath,
		ClientCrtPath: c.MqttClientCrtPath,
		ClientKeyPath: c.MqttClientKeyPath,
		CleanSession:  true,
		OnConnectHandler: func(client paho.Client) {
			log.G(ctx).Info("Connected :", clientID)
			client.Subscribe(model.BaseHeartBeatTopic, mqtt.Qos1, m.heartBeatMsgCallback)
		},
	})

	go func() {
		<-ctx.Done()
		m.mqttClient.Disconnect()
	}()

	return
}

func (m *MqttTunnel) heartBeatMsgCallback(_ paho.Client, msg paho.Message) {
	defer msg.Ack()
	baseID := getBaseIDFromTopic(msg.Topic())
	var data ArkMqttMsg[model.HeartBeatData]
	err := json.Unmarshal(msg.Payload(), &data)
	if err != nil {
		logrus.Errorf("Error unmarshalling heart beat data: %v", err)
		return
	}
	if expired(data.PublishTimestamp, 1000*10) {
		return
	}
	if m.baseDiscoveredCallback != nil {
		m.baseDiscoveredCallback(baseID, data.Data, m)
	}
}

func (m *MqttTunnel) healthMsgCallback(_ paho.Client, msg paho.Message) {
	defer msg.Ack()
	baseID := getBaseIDFromTopic(msg.Topic())
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
	if m.healthDataArrivedCallback != nil {
		m.healthDataArrivedCallback(baseID, data.Data.Data.HealthData)
	}
}

func (m *MqttTunnel) bizMsgCallback(_ paho.Client, msg paho.Message) {
	defer msg.Ack()

	baseID := getBaseIDFromTopic(msg.Topic())
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
	if m.queryAllBizDataArrivedCallback != nil {
		m.queryAllBizDataArrivedCallback(baseID, data.Data.Data)
	}
}

func (m *MqttTunnel) FetchHealthData(_ context.Context, baseID string) error {
	return m.mqttClient.Pub(formatArkletCommandTopic(baseID, model.CommandHealth), mqtt.Qos0, []byte("{}"))
}

func (m *MqttTunnel) QueryAllBizData(_ context.Context, baseID string) error {
	return m.mqttClient.Pub(formatArkletCommandTopic(baseID, model.CommandQueryAllBiz), mqtt.Qos0, []byte("{}"))
}

func (m *MqttTunnel) InstallBiz(_ context.Context, deviceID string, bizModel *ark.BizModel) error {
	installBizRequestBytes, _ := json.Marshal(bizModel)
	return m.mqttClient.Pub(formatArkletCommandTopic(deviceID, model.CommandInstallBiz), mqtt.Qos0, installBizRequestBytes)
}

func (m *MqttTunnel) UninstallBiz(_ context.Context, deviceID string, bizModel *ark.BizModel) error {
	unInstallBizRequestBytes, _ := json.Marshal(bizModel)
	return m.mqttClient.Pub(formatArkletCommandTopic(deviceID, model.CommandUnInstallBiz), mqtt.Qos0, unInstallBizRequestBytes)
}
