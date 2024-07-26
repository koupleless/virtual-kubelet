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
	"time"
)

// Defaults for root command options
const (
	DefaultNodeName             = "module-controller"
	DefaultOperatingSystem      = "linux"
	DefaultInformerResyncPeriod = 1 * time.Minute
	DefaultPodSyncWorkers       = 4
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

	// Tunnel config
	EnableMqttTunnel bool

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

	return nil
}
