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
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	klog "k8s.io/klog/v2"
)

type mapVar map[string]string

func (mv mapVar) String() string {
	var s string
	for k, v := range mv {
		if s == "" {
			s = fmt.Sprintf("%s=%v", k, v)
		} else {
			s += fmt.Sprintf(", %s=%v", k, v)
		}
	}
	return s
}

func (mv mapVar) Set(s string) error {
	split := strings.SplitN(s, "=", 2)
	if len(split) != 2 {
		return errors.Errorf("invalid format, must be `key=value`: %s", s)
	}

	_, ok := mv[split[0]]
	if ok {
		return errors.Errorf("duplicate key: %s", split[0])
	}
	mv[split[0]] = split[1]
	return nil
}

func (mv mapVar) Type() string {
	return "map"
}

func installFlags(flags *pflag.FlagSet, c *Opts) {
	flags.StringVar(&c.KubeConfigPath, "kubeconfig", c.KubeConfigPath, "kube config file to use for connecting to the Kubernetes API server")
	flags.StringVar(&c.OperatingSystem, "os", c.OperatingSystem, "Operating System (Linux/Windows)")

	flags.IntVar(&c.PodSyncWorkers, "pod-sync-workers", c.PodSyncWorkers, `set the number of pod synchronization workers`)

	flags.StringSliceVar(&c.TraceExporters, "trace-exporter", c.TraceExporters, fmt.Sprintf("sets the tracing exporter to use, available exporters: %s", AvailableTraceExporters()))
	flags.StringVar(&c.TraceConfig.ServiceName, "trace-service-name", c.TraceConfig.ServiceName, "sets the name of the service used to register with the trace exporter")
	flags.Var(mapVar(c.TraceConfig.Tags), "trace-tag", "add tags to include with traces in key=value form")
	flags.StringVar(&c.TraceSampleRate, "trace-sample-rate", c.TraceSampleRate, "set probability of tracing samples")
	flags.StringVar(&c.MqttBroker, "mqtt-broker", c.MqttBroker, "set mqtt broker")
	flags.IntVar(&c.MqttPort, "mqtt-port", c.MqttPort, "set mqtt port")
	flags.StringVar(&c.MqttUsername, "mqtt-username", c.MqttUsername, "set mqtt username")
	flags.StringVar(&c.MqttPassword, "mqtt-password", c.MqttPassword, "set mqtt password")
	flags.StringVar(&c.MqttClientPrefix, "mqtt-client-prefix", c.MqttClientPrefix, "set mqtt client prefix")
	flags.StringVar(&c.MqttCAPath, "mqtt-ca", c.MqttCAPath, "set mqtt ca path")
	flags.StringVar(&c.MqttClientCrtPath, "mqtt-client-crt", c.MqttClientCrtPath, "set mqtt client crt path")
	flags.StringVar(&c.MqttClientKeyPath, "mqtt-client-key", c.MqttClientKeyPath, "set mqtt client key path")

	flags.DurationVar(&c.InformerResyncPeriod, "full-resync-period", c.InformerResyncPeriod, "how often to perform a full resync of pods between kubernetes and the provider")

	flagset := flag.NewFlagSet("klog", flag.PanicOnError)
	klog.InitFlags(flagset)
	flagset.VisitAll(func(f *flag.Flag) {
		f.Name = "klog." + f.Name
		flags.AddGoFlag(f)
	})
}

func getEnv(key, defaultValue string) string {
	value, found := os.LookupEnv(key)
	if found {
		return value
	}
	return defaultValue
}
