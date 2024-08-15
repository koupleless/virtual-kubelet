package mqtt

import (
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/koupleless/virtual-kubelet/common/testutil/mqtt_client"
	"gotest.tools/assert"
	"testing"
	"time"
)

func TestNewMqttClient_Username(t *testing.T) {
	client, err := NewMqttClient(&ClientConfig{
		Broker:         "test-broker",
		Port:           1883,
		ClientID:       "TestNewMqttClientID",
		Username:       "test-username",
		Password:       "public",
		ClientInitFunc: mqtt_client.NewMockMqttClient,
	})
	assert.Assert(t, err == nil)
	assert.Assert(t, client != nil)
	defer client.Disconnect()
	err = client.Connect()
	assert.Assert(t, err == nil)
}

func TestNewMqttClient_CA(t *testing.T) {
	client, err := NewMqttClient(&ClientConfig{
		Broker:         "test-broker",
		Port:           8883,
		ClientID:       "TestNewMqttClientID",
		CAPath:         "../../samples/mqtt/sample-ca.crt",
		ClientInitFunc: mqtt_client.NewMockMqttClient,
	})
	defer client.Disconnect()
	assert.Assert(t, err == nil)
	assert.Assert(t, client != nil)
}

func TestNewMqttClient_CA_WithClientNotValid(t *testing.T) {
	client, err := NewMqttClient(&ClientConfig{
		Broker:         "test-broker",
		Port:           8883,
		ClientID:       "TestNewMqttClientID",
		CAPath:         "samples/sample-ca.crt",
		ClientCrtPath:  "test",
		ClientKeyPath:  "test",
		ClientInitFunc: mqtt_client.NewMockMqttClient,
	})
	assert.Assert(t, err != nil)
	assert.Assert(t, client == nil)
}

func TestClient_Pub_Sub(t *testing.T) {
	client, err := NewMqttClient(&ClientConfig{
		Broker:         "test-broker",
		Port:           1883,
		ClientID:       "TestNewMqttClientID",
		Username:       "test-username",
		Password:       "public",
		ClientInitFunc: mqtt_client.NewMockMqttClient,
	})
	assert.Assert(t, err == nil)
	assert.Assert(t, client != nil)
	err = client.Connect()
	assert.Assert(t, err == nil)
	defer client.Disconnect()
	recieved := make(chan struct{})

	success := client.Sub("topic/test/virtual-kubelet", Qos1, func(client mqtt.Client, message mqtt.Message) {
		select {
		case <-recieved:
		default:
			assert.Assert(t, string(message.Payload()) == "test-message")
			close(recieved)
		}
	})
	assert.Assert(t, success)

	success = client.Pub("topic/test/virtual-kubelet", Qos1, []byte("test-message"))
	assert.Assert(t, success)
	<-recieved
}

func TestClient_Pub_Sub_Timeout(t *testing.T) {
	client, err := NewMqttClient(&ClientConfig{
		Broker:         "test-broker",
		Port:           1883,
		ClientID:       "TestNewMqttClientID",
		Username:       "test-username",
		Password:       "public",
		ClientInitFunc: mqtt_client.NewMockMqttClient,
	})
	assert.Assert(t, err == nil)
	assert.Assert(t, client != nil)
	client.Connect()
	defer client.Disconnect()

	msgList := make([]mqtt.Message, 0)

	recieved := make(chan struct{})

	success := client.SubWithTimeout("topic/test/virtual-kubelet", Qos1, time.Second*5, func(client mqtt.Client, message mqtt.Message) {
		msgList = append(msgList, message)
		select {
		case <-recieved:
		default:
			close(recieved)
		}
	})
	assert.Assert(t, success)

	success = client.PubWithTimeout("topic/test/virtual-kubelet", Qos1, []byte("test-message"), time.Second*5)
	assert.Assert(t, success)
	<-recieved
	assert.Assert(t, len(msgList) >= 1)
}
