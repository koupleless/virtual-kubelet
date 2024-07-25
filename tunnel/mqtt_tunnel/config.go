package mqtt_tunnel

import (
	"os"
	"strconv"
)

type MqttConfig struct {
	MqttBroker        string
	MqttPort          int
	MqttUsername      string
	MqttPassword      string
	MqttClientPrefix  string
	MqttCAPath        string
	MqttClientCrtPath string
	MqttClientKeyPath string
}

func (c *MqttConfig) init() {
	if c.MqttBroker == "" {
		c.MqttBroker = os.Getenv("MQTT_BROKER")
	}

	if c.MqttPort == 0 {
		portStr := os.Getenv("MQTT_PORT")
		port, err := strconv.Atoi(portStr)
		if err == nil {
			c.MqttPort = port
		}
	}

	if c.MqttUsername == "" {
		c.MqttUsername = os.Getenv("MQTT_USERNAME")
	}

	if c.MqttPassword == "" {
		c.MqttPassword = os.Getenv("MQTT_PASSWORD")
	}

	if c.MqttClientPrefix == "" {
		c.MqttClientPrefix = os.Getenv("MQTT_CLIENT_PREFIX")
	}

	if c.MqttCAPath == "" {
		c.MqttCAPath = os.Getenv("MQTT_CA_PATH")
	}

	if c.MqttClientCrtPath == "" {
		c.MqttClientCrtPath = os.Getenv("MQTT_CLIENT_CRT_PATH")
	}

	if c.MqttClientKeyPath == "" {
		c.MqttClientKeyPath = os.Getenv("MQTT_CLIENT_KEY_PATH")
	}
}
