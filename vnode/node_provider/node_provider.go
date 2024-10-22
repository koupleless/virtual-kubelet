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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sync"
)

var _ virtual_kubelet.NodeProvider = &VNodeProvider{}

type VNodeProvider struct {
	sync.Mutex

	nodeConfig *model.BuildVNodeProviderConfig

	nodeInfo *corev1.Node

	latestNodeStatusData model.NodeStatusData

	notify func(*corev1.Node)
}

func (v *VNodeProvider) constructVNode() *corev1.Node {
	vnodeCopy := v.nodeInfo.DeepCopy()
	// node status
	vnodeCopy.Status.Phase = corev1.NodeRunning
	conditionMap := map[corev1.NodeConditionType]corev1.NodeCondition{
		corev1.NodeReady: {
			Type:   corev1.NodeReady,
			Status: corev1.ConditionTrue,
		},
		corev1.NodeMemoryPressure: {
			Type:   corev1.NodeMemoryPressure,
			Status: corev1.ConditionFalse,
		},
		corev1.NodeDiskPressure: {
			Type:   corev1.NodeDiskPressure,
			Status: corev1.ConditionFalse,
		},
		corev1.NodePIDPressure: {
			Type:   corev1.NodePIDPressure,
			Status: corev1.ConditionFalse,
		},
		corev1.NodeNetworkUnavailable: {
			Type:   corev1.NodeNetworkUnavailable,
			Status: corev1.ConditionFalse,
		},
	}

	for _, customCondition := range v.latestNodeStatusData.CustomConditions {
		conditionMap[customCondition.Type] = customCondition
	}

	conditions := make([]corev1.NodeCondition, 0)
	for _, condition := range conditionMap {
		conditions = append(conditions, condition)
	}
	vnodeCopy.Status.Conditions = conditions
	vnodeCopy.Annotations = v.latestNodeStatusData.CustomAnnotations
	for key, value := range v.latestNodeStatusData.CustomLabels {
		vnodeCopy.Labels[key] = value
	}
	for resourceName, status := range v.latestNodeStatusData.Resources {
		vnodeCopy.Status.Capacity[resourceName] = status.Capacity
		vnodeCopy.Status.Allocatable[resourceName] = status.Allocatable
	}
	return vnodeCopy
}

func (v *VNodeProvider) Notify(data model.NodeStatusData) {
	v.Lock()
	defer v.Unlock()
	v.latestNodeStatusData = data
	vnodeCopy := v.constructVNode()
	v.notify(vnodeCopy)
}

func (v *VNodeProvider) CurrNodeInfo() *corev1.Node {
	v.Lock()
	defer v.Unlock()
	return v.constructVNode()
}

func NewVirtualKubeletNode(config model.BuildVNodeProviderConfig) *VNodeProvider {
	return &VNodeProvider{
		nodeConfig: &config,
	}
}

func (v *VNodeProvider) BuildVirtualNode(node *corev1.Node, tunnelKey string) {
	config := *v.nodeConfig
	// custom labels
	node.Labels = v.nodeConfig.CustomLabels
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	// necessary labels, will cover the value of custom labels
	node.Labels[model.LabelKeyOfVNodeVersion] = config.Version
	node.Labels[model.LabelKeyOfVNodeName] = config.Name
	node.Labels[model.LabelKeyOfEnv] = config.Env
	node.Labels[model.LabelKeyOfComponent] = model.ComponentVNode
	node.Labels[model.LabelKeyOfVnodeTunnel] = tunnelKey

	node.Annotations = v.nodeConfig.CustomAnnotations
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}

	node.Spec.Taints = append([]corev1.Taint{
		{
			Key:    model.TaintKeyOfVnode,
			Value:  "True",
			Effect: corev1.TaintEffectNoExecute,
		},
		{
			Key:    model.TaintKeyOfEnv,
			Value:  config.Env,
			Effect: corev1.TaintEffectNoExecute,
		},
	}, config.CustomTaints...)
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

func (v *VNodeProvider) Register(node *corev1.Node, tunnelKey string) error {
	v.BuildVirtualNode(node, tunnelKey)
	v.Lock()
	v.nodeInfo = node.DeepCopy()
	v.Unlock()
	return nil
}

func (v *VNodeProvider) Ping(_ context.Context) error {
	return nil
}

func (v *VNodeProvider) NotifyNodeStatus(_ context.Context, cb func(*corev1.Node)) {
	v.notify = cb
}
