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

package node_provider

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sync"
	"time"
)

type NodeProvider interface {
	virtual_kubelet.NodeProvider

	// Register configure node on first attempt
	Register(ctx context.Context, node *corev1.Node) error
}

var _ NodeProvider = &BaseNodeProvider{}

type BaseNodeProvider struct {
	sync.Mutex
	nodeConfig *BuildBaseNodeProviderConfig

	nodeInfo *corev1.Node

	notify func(*corev1.Node)
}

func (v *BaseNodeProvider) Notify(data ark.HealthData) {
	v.Lock()
	defer v.Unlock()
	if v.nodeInfo == nil {
		return
	}
	// node status
	nodeReadyStatus := corev1.ConditionTrue
	nodeReadyMessage := ""
	v.nodeInfo.Status.Phase = corev1.NodeRunning
	conditions := []corev1.NodeCondition{
		{
			Type:   corev1.NodeReady,
			Status: nodeReadyStatus,
			LastHeartbeatTime: metav1.Time{
				Time: time.Now(),
			},
			Message: nodeReadyMessage,
		},
	}
	v.nodeInfo.Status.Conditions = conditions
	if data.Jvm.JavaMaxMetaspace != -1 {
		v.nodeInfo.Status.Capacity[corev1.ResourceMemory] = utils.ConvertByteNumToResourceQuantity(data.Jvm.JavaMaxMetaspace)
	}
	if data.Jvm.JavaCommittedMetaspace != -1 && data.Jvm.JavaMaxMetaspace != -1 {
		v.nodeInfo.Status.Allocatable[corev1.ResourceMemory] = utils.ConvertByteNumToResourceQuantity(data.Jvm.JavaMaxMetaspace - data.Jvm.JavaCommittedMetaspace)
	}
	v.notify(v.nodeInfo.DeepCopy())
}

func (v *BaseNodeProvider) CurrNodeInfo() *corev1.Node {
	v.Lock()
	defer v.Unlock()
	return v.nodeInfo.DeepCopy()
}

func NewVirtualKubeletNode(config BuildBaseNodeProviderConfig) *BaseNodeProvider {
	return &BaseNodeProvider{
		nodeConfig: &config,
		notify: func(_ *corev1.Node) {
			// default notify func
			log.G(context.Background()).Info("node status callback not registered")
		},
	}
}

func (v *BaseNodeProvider) BuildVirtualNode(node *corev1.Node) {
	config := *v.nodeConfig
	if node.ObjectMeta.Labels == nil {
		node.ObjectMeta.Labels = make(map[string]string)
	}
	node.Labels["base.koupleless.io/stack"] = config.TechStack
	node.Labels["base.koupleless.io/version"] = config.Version
	node.Labels["base.koupleless.io/name"] = config.BizName
	node.Spec.Taints = []corev1.Taint{
		{
			Key:    "schedule.koupleless.io/virtual-node",
			Value:  "True",
			Effect: corev1.TaintEffectNoExecute,
		},
	}
	node.Status = corev1.NodeStatus{
		Phase: corev1.NodePending,
		Addresses: []corev1.NodeAddress{
			{
				Type:    corev1.NodeInternalIP,
				Address: config.NodeIP,
			},
		},
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionFalse,
			},
		},
		Capacity: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourcePods: resource.MustParse("2000"),
		},
		Allocatable: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourcePods: resource.MustParse("2000"),
		},
	}
}

func (v *BaseNodeProvider) Register(_ context.Context, node *corev1.Node) error {
	v.BuildVirtualNode(node)
	v.Lock()
	v.nodeInfo = node.DeepCopy()
	v.Unlock()
	return nil
}

func (v *BaseNodeProvider) Ping(_ context.Context) error {
	return nil
}

func (v *BaseNodeProvider) NotifyNodeStatus(_ context.Context, cb func(*corev1.Node)) {
	v.notify = cb
}
