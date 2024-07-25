package mqtt

import (
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"gotest.tools/assert"
	"testing"
	"time"
)

func TestNewMqttClient_Username(t *testing.T) {
	client, err := NewMqttClient(&ClientConfig{
		Broker:   "broker.emqx.io",
		Port:     1883,
		ClientID: "TestNewMqttClientID",
		Username: "emqx",
		Password: "public",
	})
	assert.Assert(t, err == nil)
	assert.Assert(t, client != nil)
}

func TestNewMqttClient_CA(t *testing.T) {
	client, err := NewMqttClient(&ClientConfig{
		Broker:   "broker.emqx.io",
		Port:     8883,
		ClientID: "TestNewMqttClientID",
		CAPath:   "../../samples/mqtt/sample-ca.crt",
	})
	if err != nil {
		fmt.Println(err.Error())
	}
	assert.Assert(t, err == nil)
	assert.Assert(t, client != nil)
}

func TestNewMqttClient_CA_WithClient(t *testing.T) {
	client, err := NewMqttClient(&ClientConfig{
		Broker:        "broker.emqx.io",
		Port:          8883,
		ClientID:      "TestNewMqttClientID",
		CAPath:        "samples/sample-ca.crt",
		ClientCrtPath: "test",
		ClientKeyPath: "test",
	})
	assert.Assert(t, err != nil)
	assert.Assert(t, client == nil)
}

func TestClient_Pub_Sub(t *testing.T) {
	client, err := NewMqttClient(&ClientConfig{
		Broker:   "broker.emqx.io",
		Port:     1883,
		ClientID: "TestNewMqttClientID",
		Username: "emqx",
		Password: "public",
	})
	if err != nil {
		fmt.Println(err.Error())
	}
	assert.Assert(t, err == nil)
	assert.Assert(t, client != nil)

	recieved := make(chan struct{})

	success := client.Sub("topic/test/virtual-kubelet", Qos1, func(client mqtt.Client, message mqtt.Message) {
		select {
		case <-recieved:
			assert.Assert(t, string(message.Payload()) == "test-message")
		default:
			close(recieved)
		}
	})
	assert.Assert(t, success)

	success = client.Pub("topic/test/virtual-kubelet", Qos1, "test-message")
	assert.Assert(t, success)
	<-recieved
}

func TestClient_Pub_Sub_Timeout(t *testing.T) {
	client, err := NewMqttClient(&ClientConfig{
		Broker:   "broker.emqx.io",
		Port:     1883,
		ClientID: "TestNewMqttClientID",
		Username: "emqx",
		Password: "public",
	})
	assert.Assert(t, err == nil)
	assert.Assert(t, client != nil)

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

	success = client.PubWithTimeout("topic/test/virtual-kubelet", Qos1, "test-message", time.Second*5)
	assert.Assert(t, success)
	<-recieved
	assert.Assert(t, len(msgList) >= 1)
}
