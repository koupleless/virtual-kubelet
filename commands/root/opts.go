// Copyright © 2017 The virtual-kubelet authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package root

import (
	"os"
	"strconv"
	"time"
)

// Defaults for root command options
const (
	DefaultNodeName             = "module-controller"
	DefaultOperatingSystem      = "linux"
	DefaultInformerResyncPeriod = 1 * time.Minute
	DefaultPodSyncWorkers       = 10
)

// Opts stores all the options for configuring the root module-controller command.
// It is used for setting flag values.
//
// You can set the default options by creating a new `Opts` struct and passing
// it into `SetDefaultOpts`
type Opts struct {
	// Path to the kubeconfig to use to connect to the Kubernetes API server.
	KubeConfigPath string
	// Operating system to run pods for
	OperatingSystem string

	// Number of workers to use to handle pod notifications
	PodSyncWorkers       int
	InformerResyncPeriod time.Duration

	TraceExporters  []string
	TraceSampleRate string
	TraceConfig     TracingExporterOptions

	// MQTT config
	MqttBroker        string
	MqttPort          int
	MqttUsername      string
	MqttPassword      string
	MqttClientPrefix  string
	MqttCAPath        string
	MqttClientCrtPath string
	MqttClientKeyPath string

	Version string
}

// SetDefaultOpts sets default options for unset values on the passed in option struct.
// Fields tht are already set will not be modified.
func SetDefaultOpts(c *Opts) error {
	if c.OperatingSystem == "" {
		c.OperatingSystem = DefaultOperatingSystem
	}

	if c.InformerResyncPeriod == 0 {
		c.InformerResyncPeriod = DefaultInformerResyncPeriod
	}

	if c.PodSyncWorkers == 0 {
		c.PodSyncWorkers = DefaultPodSyncWorkers
	}

	if c.TraceConfig.ServiceName == "" {
		c.TraceConfig.ServiceName = DefaultNodeName
	}

	if c.KubeConfigPath == "" {
		c.KubeConfigPath = os.Getenv("KUBE_CONFIG_PATH")
	}

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

	return nil
}
