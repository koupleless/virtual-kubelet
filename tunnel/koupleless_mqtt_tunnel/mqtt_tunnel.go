package koupleless_mqtt_tunnel

import (
	"context"
	"encoding/json"
	"fmt"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/mqtt"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
)

var _ tunnel.Tunnel = &MqttTunnel{}

type MqttTunnel struct {
	mqttClient *mqtt.Client
	env        string

	ready bool

	onBaseDiscovered              tunnel.OnNodeDiscovered
	onHealthDataArrived           tunnel.OnNodeStatusDataArrived
	onQueryAllBizDataArrived      tunnel.OnQueryAllContainerStatusDataArrived
	onInstallBizResponseArrived   tunnel.OnStartContainerResponseArrived
	onUninstallBizResponseArrived tunnel.OnShutdownContainerResponseArrived
	queryBaseline                 tunnel.QueryContainersBaseline
}

func (m *MqttTunnel) Ready() bool {
	return m.ready
}

func (m *MqttTunnel) GetContainerUniqueKey(_ string, container *corev1.Container) string {
	return getBizIdentity(container.Name, getBizVersionFromContainer(container))
}

func (m *MqttTunnel) OnNodeStart(ctx context.Context, baseID string) {
	err := m.mqttClient.Sub(fmt.Sprintf(BaseHealthTopic, m.env, baseID), mqtt.Qos1, m.healthMsgCallback)
	if err != nil {
		log.G(ctx).Error(err.Error())
	}
	err = m.mqttClient.Sub(fmt.Sprintf(BaseBizTopic, m.env, baseID), mqtt.Qos1, m.bizMsgCallback)
	if err != nil {
		log.G(ctx).Error(err.Error())
	}
	err = m.mqttClient.Sub(fmt.Sprintf(BaseBizOperationResponseTopic, m.env, baseID), mqtt.Qos1, m.bizOperationResponseCallback)
	if err != nil {
		log.G(ctx).Error(err.Error())
	}
}

func (m *MqttTunnel) OnNodeStop(ctx context.Context, baseID string) {
	err := m.mqttClient.UnSub(fmt.Sprintf(BaseHealthTopic, m.env, baseID))
	if err != nil {
		log.G(ctx).Error(err.Error())
	}
	err = m.mqttClient.UnSub(fmt.Sprintf(BaseBizTopic, m.env, baseID))
	if err != nil {
		log.G(ctx).Error(err.Error())
	}
	err = m.mqttClient.UnSub(fmt.Sprintf(BaseBizOperationResponseTopic, m.env, baseID))
	if err != nil {
		log.G(ctx).Error(err.Error())
	}
}

func (m *MqttTunnel) Key() string {
	return "mqtt_tunnel_provider"
}

func (m *MqttTunnel) RegisterCallback(onBaseDiscovered tunnel.OnNodeDiscovered, onHealthDataArrived tunnel.OnNodeStatusDataArrived, onQueryAllBizDataArrived tunnel.OnQueryAllContainerStatusDataArrived, onInstallBizResponseArrived tunnel.OnStartContainerResponseArrived, onUnInstallBizResponseArrived tunnel.OnShutdownContainerResponseArrived) {
	m.onBaseDiscovered = onBaseDiscovered

	m.onHealthDataArrived = onHealthDataArrived

	m.onQueryAllBizDataArrived = onQueryAllBizDataArrived

	m.onInstallBizResponseArrived = onInstallBizResponseArrived

	m.onUninstallBizResponseArrived = onUnInstallBizResponseArrived
}

func (m *MqttTunnel) RegisterQuery(queryBaseline tunnel.QueryContainersBaseline) {
	m.queryBaseline = queryBaseline
}

func (m *MqttTunnel) Start(ctx context.Context, clientID, env string) (err error) {
	c := &MqttConfig{}
	c.init()
	clientID = fmt.Sprintf("%s@@@%s", c.MqttClientPrefix, clientID)
	m.env = env
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
			log.G(ctx).Info("MQTT client connected :", clientID)
			client.Subscribe(fmt.Sprintf(BaseHeartBeatTopic, m.env), mqtt.Qos1, m.heartBeatMsgCallback)
			client.Subscribe(fmt.Sprintf(BaseQueryBaselineTopic, m.env), mqtt.Qos1, m.queryBaselineMsgCallback)
		},
	})

	err = m.mqttClient.Connect()
	if err != nil {
		log.G(ctx).WithError(err).Error("mqtt connect error")
		return
	}

	go func() {
		<-ctx.Done()
		m.mqttClient.Disconnect()
	}()
	m.ready = true
	return
}

func (m *MqttTunnel) heartBeatMsgCallback(_ paho.Client, msg paho.Message) {
	defer msg.Ack()
	baseID := getBaseIDFromTopic(msg.Topic())
	var data ArkMqttMsg[HeartBeatData]
	err := json.Unmarshal(msg.Payload(), &data)
	if err != nil {
		logrus.Errorf("Error unmarshalling heart beat data: %v", err)
		return
	}
	if expired(data.PublishTimestamp, 1000*10) {
		return
	}
	if m.onBaseDiscovered != nil {
		m.onBaseDiscovered(baseID, translateHeartBeatDataToNodeInfo(data.Data), m)
	}
}

func (m *MqttTunnel) queryBaselineMsgCallback(_ paho.Client, msg paho.Message) {
	defer msg.Ack()
	baseID := getBaseIDFromTopic(msg.Topic())
	var data ArkMqttMsg[ark.MasterBizInfo]
	err := json.Unmarshal(msg.Payload(), &data)
	if err != nil {
		logrus.Errorf("Error unmarshalling heart beat data: %v", err)
		return
	}
	if expired(data.PublishTimestamp, 1000*10) {
		return
	}
	if m.queryBaseline != nil {
		baseline := m.queryBaseline(translateHeartBeatDataToBaselineQuery(data.Data))
		go func() {
			baselineBytes, _ := json.Marshal(baseline)
			err = m.mqttClient.Pub(formatBaselineResponseTopic(m.env, baseID), mqtt.Qos1, baselineBytes)
			if err != nil {
				logrus.WithError(err).Errorf("Error publishing baseline response data")
			}
		}()
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
	if m.onHealthDataArrived != nil {
		m.onHealthDataArrived(baseID, translateHealthDataToNodeStatus(data.Data.Data.HealthData))
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
	if m.onQueryAllBizDataArrived != nil {
		m.onQueryAllBizDataArrived(baseID, translateQueryAllBizDataToContainerStatuses(data.Data.Data))
	}
}

func (m *MqttTunnel) bizOperationResponseCallback(_ paho.Client, msg paho.Message) {
	defer msg.Ack()

	baseID := getBaseIDFromTopic(msg.Topic())
	var data ArkMqttMsg[BizOperationResponse]
	err := json.Unmarshal(msg.Payload(), &data)
	if err != nil {
		logrus.Errorf("Error unmarshalling biz response: %v", err)
		return
	}
	if expired(data.PublishTimestamp, 1000*10) {
		return
	}

	if data.Data.Command == CommandInstallBiz {
		if m.onInstallBizResponseArrived != nil {
			m.onInstallBizResponseArrived(baseID, translateCommandResponseToContainerOperationResponseData(data.Data))
		}
	} else if data.Data.Command == CommandUnInstallBiz {
		if m.onUninstallBizResponseArrived != nil {
			m.onUninstallBizResponseArrived(baseID, translateCommandResponseToContainerOperationResponseData(data.Data))
		}
	}
}

func (m *MqttTunnel) FetchHealthData(_ context.Context, baseID string) error {
	return m.mqttClient.Pub(formatArkletCommandTopic(m.env, baseID, CommandHealth), mqtt.Qos0, []byte("{}"))
}

func (m *MqttTunnel) QueryAllContainerStatusData(_ context.Context, baseID string) error {
	return m.mqttClient.Pub(formatArkletCommandTopic(m.env, baseID, CommandQueryAllBiz), mqtt.Qos0, []byte("{}"))
}

func (m *MqttTunnel) StartContainer(_ context.Context, deviceID, podKey string, container *corev1.Container) error {
	installBizRequestBytes, _ := json.Marshal(translateCoreV1ContainerToBizModel(container))
	return m.mqttClient.Pub(formatArkletCommandTopic(m.env, deviceID, CommandInstallBiz), mqtt.Qos0, installBizRequestBytes)
}

func (m *MqttTunnel) ShutdownContainer(_ context.Context, deviceID, podKey string, container *corev1.Container) error {
	unInstallBizRequestBytes, _ := json.Marshal(translateCoreV1ContainerToBizModel(container))
	return m.mqttClient.Pub(formatArkletCommandTopic(m.env, deviceID, CommandUnInstallBiz), mqtt.Qos0, unInstallBizRequestBytes)
}
