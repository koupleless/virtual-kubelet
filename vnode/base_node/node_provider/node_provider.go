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
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sync"
	"time"
)

var _ virtual_kubelet.NodeProvider = &BaseNodeProvider{}

type BaseNodeProvider struct {
	sync.Mutex

	nodeConfig *BuildBaseNodeProviderConfig

	nodeInfo *corev1.Node

	latestNodeStatusData model.NodeStatusData

	notify func(*corev1.Node)
}

func (v *BaseNodeProvider) constructVNode() *corev1.Node {
	vnodeCopy := v.nodeInfo.DeepCopy()
	// node status
	vnodeCopy.Status.Phase = corev1.NodeRunning
	conditions := []corev1.NodeCondition{
		{
			Type:   corev1.NodeReady,
			Status: corev1.ConditionTrue,
			LastHeartbeatTime: metav1.Time{
				Time: time.Now(),
			},
		},
	}
	for _, condition := range v.latestNodeStatusData.CustomConditions {
		if condition.Type == corev1.NodeReady {
			conditions[0].Status = condition.Status
		} else {
			conditions = append(conditions, condition)
		}
	}
	vnodeCopy.Status.Conditions = conditions
	vnodeCopy.Annotations = v.latestNodeStatusData.CustomAnnotations
	for key, value := range v.latestNodeStatusData.CustomLabels {
		vnodeCopy.Labels[key] = value
	}
	vnodeCopy.Spec.Taints = append(vnodeCopy.Spec.Taints, v.latestNodeStatusData.CustomTaints...)
	for resourceName, status := range v.latestNodeStatusData.Resources {
		vnodeCopy.Status.Capacity[resourceName] = status.Capacity
		vnodeCopy.Status.Allocatable[resourceName] = status.Allocatable
	}
	return vnodeCopy
}

func (v *BaseNodeProvider) Notify(data model.NodeStatusData) {
	v.Lock()
	defer v.Unlock()
	v.latestNodeStatusData = data
	vnodeCopy := v.constructVNode()
	if vnodeCopy == nil {
		return
	}
	v.notify(vnodeCopy)
}

func (v *BaseNodeProvider) CurrNodeInfo() *corev1.Node {
	v.Lock()
	defer v.Unlock()
	return v.constructVNode()
}

func NewVirtualKubeletNode(config BuildBaseNodeProviderConfig) *BaseNodeProvider {
	return &BaseNodeProvider{
		nodeConfig: &config,
		notify: func(_ *corev1.Node) {
			// default notify func
			logrus.Info("node status callback not registered")
		},
	}
}

func (v *BaseNodeProvider) BuildVirtualNode(node *corev1.Node, tunnelKey string) {
	config := *v.nodeConfig
	if node.ObjectMeta.Labels == nil {
		node.ObjectMeta.Labels = make(map[string]string)
	}
	// custom labels
	node.Labels[model.LabelKeyOfVNodeVersion] = config.Version
	node.Labels[model.LabelKeyOfVNodeName] = config.Name
	// necessary labels
	node.Labels[model.LabelKeyOfEnv] = config.Env
	node.Labels[model.LabelKeyOfScheduleAnythingComponent] = model.ComponentVNode
	node.Labels[model.LabelKeyOfVnodeTunnel] = tunnelKey
	node.Spec.Taints = []corev1.Taint{
		{
			Key:    "schedule.koupleless.io/virtual-node",
			Value:  "True",
			Effect: corev1.TaintEffectNoExecute,
		},
		{
			Key:    "schedule.koupleless.io/node-env",
			Value:  config.Env,
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
			{
				Type:    corev1.NodeHostName,
				Address: config.NodeHostname,
			},
		},
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionFalse,
			},
		},
		Capacity: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourcePods: resource.MustParse("65535"),
		},
		Allocatable: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourcePods: resource.MustParse("65535"),
		},
	}
}

func (v *BaseNodeProvider) Register(node *corev1.Node, tunnelKey string) error {
	v.BuildVirtualNode(node, tunnelKey)
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
