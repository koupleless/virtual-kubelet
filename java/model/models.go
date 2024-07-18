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
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/listers/core/v1"
	"time"
)

const (
	CommandHealth       = "health"
	CommandQueryAllBiz  = "queryAllBiz"
	CommandInstallBiz   = "installBiz"
	CommandUnInstallBiz = "uninstallBiz"
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

	// K8SConfig is the config of k8s client
	K8SConfig *K8SConfig
}

type K8SConfig struct {
	// KubeConfigPath is the path of k8s client
	KubeConfigPath string

	// InformerSyncPeriod
	InformerSyncPeriod time.Duration
}

type BuildKouplelessNodeConfig struct {
	// KubeClient is the kube client instance
	KubeClient *kubernetes.Clientset

	// PodLister is the pod lister
	PodLister v1.PodLister

	// MqttClient is the mqtt client, for sub and pub
	MqttClient *mqtt.Client

	// NodeID is the base id of base
	NodeID string

	// NodeIP is the base ip of base
	NodeIP string

	// TechStack is the base tech stack, default java
	TechStack string

	// BizName is the base master biz name
	BizName string

	// BizVersion is the base master biz version
	BizVersion string
}
