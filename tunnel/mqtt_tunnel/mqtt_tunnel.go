package mqtt_tunnel

import (
	"context"
	"encoding/json"
	"errors"
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

func (m *MqttTunnel) Name() string {
	return "mqtt_tunnel_provider"
}

func (m *MqttTunnel) Register(ctx context.Context, clientID string, baseDiscoveredCallback tunnel.BaseDiscoveredCallback, healthDataArrivedCallback tunnel.HealthDataArrivedCallback, queryAllBizDataArrivedCallback tunnel.QueryAllBizDataArrivedCallback) (err error) {
	if baseDiscoveredCallback == nil {
		return errors.New("base discovered callback is nil")
	}
	m.baseDiscoveredCallback = baseDiscoveredCallback

	if healthDataArrivedCallback == nil {
		return errors.New("health data arrived callback is nil")
	}
	m.healthDataArrivedCallback = healthDataArrivedCallback

	if queryAllBizDataArrivedCallback == nil {
		return errors.New("query all data arrived callback is nil")
	}
	m.queryAllBizDataArrivedCallback = queryAllBizDataArrivedCallback

	c := MqttConfig{}
	c.init()
	m.mqttClient, err = mqtt.NewMqttClient(&mqtt.ClientConfig{
		Broker:        c.MqttBroker,
		Port:          c.MqttPort,
		ClientID:      fmt.Sprintf("%s@@@%s", c.MqttClientPrefix, clientID),
		Username:      c.MqttUsername,
		Password:      c.MqttPassword,
		CAPath:        c.MqttCAPath,
		ClientCrtPath: c.MqttClientCrtPath,
		ClientKeyPath: c.MqttClientKeyPath,
		CleanSession:  true,
		OnConnectHandler: func(client paho.Client) {
			log.G(ctx).Info("Connected")
			reader := client.OptionsReader()
			log.G(ctx).Info("Connect options: ", reader.ClientID())
			client.Subscribe(model.BaseHeartBeatTopic, mqtt.Qos1, m.heartBeatMsgCallback)
			client.Subscribe(model.BaseHealthTopic, mqtt.Qos1, m.healthMsgCallback)
			client.Subscribe(model.BaseBizTopic, mqtt.Qos1, m.bizMsgCallback)
		},
	})

	if err != nil {
		return
	}
	if m.mqttClient == nil {
		return errors.New("mqtt client is nil")
	}

	return
}

func (m *MqttTunnel) heartBeatMsgCallback(_ paho.Client, msg paho.Message) {
	if msg.Qos() > 0 {
		defer msg.Ack()
	}
	baseID := getBaseIDFromTopic(msg.Topic())
	if baseID == "" {
		logrus.Error("Received non device heart beat msg from topic: ", msg.Topic())
		return
	}
	var data ArkMqttMsg[model.HeartBeatData]
	err := json.Unmarshal(msg.Payload(), &data)
	if err != nil {
		logrus.Errorf("Error unmarshalling heart beat data: %v", err)
		return
	}
	if expired(data.PublishTimestamp, 1000*10) {
		return
	}

	m.baseDiscoveredCallback(baseID, data.Data, m)
}

func (m *MqttTunnel) healthMsgCallback(_ paho.Client, msg paho.Message) {
	if msg.Qos() > 0 {
		defer msg.Ack()
	}
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

	go func() {
		if expired(data.PublishTimestamp, 1000*10) {
			return
		}
		if data.Data.Code != "SUCCESS" {
			return
		}
		m.healthDataArrivedCallback(baseID, data.Data.Data.HealthData)
	}()
}

func (m *MqttTunnel) bizMsgCallback(_ paho.Client, msg paho.Message) {
	if msg.Qos() > 0 {
		defer msg.Ack()
	}
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

	go func() {
		if expired(data.PublishTimestamp, 1000*10) {
			return
		}
		if data.Data.Code != "SUCCESS" {
			return
		}

	}()
}

func (m *MqttTunnel) FetchHealthData(_ context.Context, baseID string) error {
	success := m.mqttClient.Pub(formatArkletCommandTopic(baseID, model.CommandHealth), mqtt.Qos0, "{}")
	if !success {
		return errors.New("mqtt client pub health command failed")
	}
	return nil
}

func (m *MqttTunnel) QueryAllBizData(_ context.Context, baseID string) error {
	success := m.mqttClient.Pub(formatArkletCommandTopic(baseID, model.CommandQueryAllBiz), mqtt.Qos0, "{}")
	if !success {
		return errors.New("mqtt client pub queryAllBiz command failed")
	}
	return nil
}

func (m *MqttTunnel) InstallBiz(_ context.Context, deviceID string, bizModel *ark.BizModel) error {
	installBizRequestBytes, _ := json.Marshal(bizModel)
	success := m.mqttClient.Pub(formatArkletCommandTopic(deviceID, model.CommandInstallBiz), mqtt.Qos1, installBizRequestBytes)
	if !success {
		return errors.New("install biz command publish failed")
	}
	return nil
}

func (m *MqttTunnel) UninstallBiz(_ context.Context, deviceID string, bizModel *ark.BizModel) error {
	unInstallBizRequestBytes, _ := json.Marshal(bizModel)
	success := m.mqttClient.Pub(formatArkletCommandTopic(deviceID, model.CommandUnInstallBiz), mqtt.Qos1, unInstallBizRequestBytes)
	if !success {
		return errors.New("uninstall biz command publish failed")
	}
	return nil
}
