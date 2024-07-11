/**
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package model

import (
	"github.com/koupleless/virtual-kubelet/common/mqtt"
)

const (
	CommandHealth       = "health"
	CommandQueryAllBiz  = "queryAllBiz"
	CommandInstallBiz   = "installBiz"
	CommandUnInstallBiz = "uninstallBiz"
)

type contextKey string

const (
	TimedTaskNameKey contextKey = "TimedTaskName"
)

type BuildVirtualNodeConfig struct {
	// NodeIP is the ip of the node
	NodeIP string `json:"nodeIP"`

	// TechStack is the underlying tech stack of runtime
	TechStack string `json:"techStack"`

	// BizName is the master biz name of runtime
	BizName string `json:"bizName"`

	// Version is the version of ths underlying runtime
	Version string `json:"version"`
}

type BuildBaseRegisterControllerConfig struct {
	// MqttConfig is the config of mqtt client
	MqttConfig *mqtt.ClientConfig

	// KubeConfigPath is the path of k8s client
	KubeConfigPath string
}

type BuildKouplelessNodeConfig struct {
	// KubeConfigPath is the path of kube config file
	KubeConfigPath string

	// MqttClient is the mqtt client, for sub and pub
	MqttClient *mqtt.Client

	// NodeID is the device id of base
	NodeID string

	// NodeIP is the device ip of base
	NodeIP string

	// TechStack is the base tech stack, default java
	TechStack string

	// BizName is the base master biz name
	BizName string

	// BizVersion is the base master biz version
	BizVersion string
}
