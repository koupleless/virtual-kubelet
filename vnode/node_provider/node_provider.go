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
	"sync"

	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// The following line ensures that VNodeProvider implements the NodeProvider interface.
var _ virtual_kubelet.NodeProvider = &VNodeProvider{}

// VNodeProvider is a struct that implements the NodeProvider interface.
type VNodeProvider struct {
	sync.Mutex

	nodeConfig *model.BuildVNodeProviderConfig // Configuration for building a virtual node provider.

	nodeInfo *corev1.Node // Information about the node.

	latestNodeStatusData model.NodeStatusData // The latest status data of the node.

	notify func(*corev1.Node) // Function to notify about node status changes.
}

// constructVNode constructs a virtual node based on the latest node status data.
func (v *VNodeProvider) constructVNode() *corev1.Node {
	vnodeCopy := v.nodeInfo.DeepCopy() // Create a deep copy of the node info.
	// Set the node status to running.
	vnodeCopy.Status.Phase = corev1.NodeRunning
	// Initialize a map to hold node conditions.
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

	// Add custom conditions to the condition map.
	for _, customCondition := range v.latestNodeStatusData.CustomConditions {
		conditionMap[customCondition.Type] = customCondition
	}

	// Convert the condition map to a slice of conditions.
	conditions := make([]corev1.NodeCondition, 0)
	for _, condition := range conditionMap {
		conditions = append(conditions, condition)
	}
	vnodeCopy.Status.Conditions = conditions                         // Set the conditions on the vnode copy.
	vnodeCopy.Annotations = v.latestNodeStatusData.CustomAnnotations // Set custom annotations.
	// Set custom labels.
	for key, value := range v.latestNodeStatusData.CustomLabels {
		vnodeCopy.Labels[key] = value
	}
	// Set resource capacities and allocatable amounts.
	for resourceName, status := range v.latestNodeStatusData.Resources {
		vnodeCopy.Status.Capacity[resourceName] = status.Capacity
		vnodeCopy.Status.Allocatable[resourceName] = status.Allocatable
	}
	return vnodeCopy // Return the constructed vnode.
}

// Notify updates the latest node status data and notifies about the change.
func (v *VNodeProvider) Notify(data model.NodeStatusData) {
	v.Lock()
	defer v.Unlock()
	v.latestNodeStatusData = data
	vnodeCopy := v.constructVNode()
	v.notify(vnodeCopy)
}

// CurrNodeInfo returns the current node information.
func (v *VNodeProvider) CurrNodeInfo() *corev1.Node {
	v.Lock()
	defer v.Unlock()
	return v.constructVNode()
}

// NewVirtualKubeletNode creates a new VNodeProvider instance.
func NewVirtualKubeletNode(config model.BuildVNodeProviderConfig) *VNodeProvider {
	return &VNodeProvider{
		nodeConfig: &config,
	}
}

// BuildVirtualNode builds a virtual node based on the provided configuration.
func (v *VNodeProvider) BuildVirtualNode(node *corev1.Node) {
	config := *v.nodeConfig // Copy the node configuration.
	// Set custom labels on the node.
	node.Labels = v.nodeConfig.CustomLabels
	if node.Labels == nil {
		node.Labels = make(map[string]string) // Initialize labels if not present.
	}

	// Set necessary labels on the node.
	node.Labels[model.LabelKeyOfVNodeVersion] = config.Version
	node.Labels[model.LabelKeyOfVNodeName] = config.Name
	node.Labels[model.LabelKeyOfEnv] = config.Env
	node.Labels[model.LabelKeyOfComponent] = model.ComponentVNode

	// Set custom annotations on the node.
	node.Annotations = v.nodeConfig.CustomAnnotations
	if node.Annotations == nil {
		node.Annotations = make(map[string]string) // Initialize annotations if not present.
	}

	// Add taints to the node.
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

	// Set the node status.
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

// Register registers a node with the provider.
func (v *VNodeProvider) Register(node *corev1.Node) error {
	v.BuildVirtualNode(node)
	v.Lock()
	v.nodeInfo = node.DeepCopy()
	v.Unlock()
	return nil
}

// Ping checks the health of the node provider.
func (v *VNodeProvider) Ping(_ context.Context) error {
	return nil
}

// NotifyNodeStatus sets a callback function to notify about node status changes.
func (v *VNodeProvider) NotifyNodeStatus(_ context.Context, cb func(*corev1.Node)) {
	v.notify = cb
}
