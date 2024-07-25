package mqtt_broker

import (
	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"sync"
)

var lock sync.Mutex

var started = false

func StartLocalMqttBroker() {
	lock.Lock()
	defer lock.Unlock()
	if started {
		return
	}
	started = true
	// Create the new MQTT Server.
	server := mqtt.New(nil)

	// Allow all connections.
	_ = server.AddHook(new(auth.AllowHook), nil)

	// Create a TCP listener on a standard port.
	tcp := listeners.NewTCP(listeners.Config{ID: "t1", Address: ":1883"})
	err := server.AddListener(tcp)
	if err != nil {
		return
	}

	err = server.Serve()
	if err != nil {
		return
	}
}
