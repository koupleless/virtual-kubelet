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

package vnode_controller

import (
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/vnode"
	"sync"
)

// Summary:
// This file defines the RuntimeInfoStore structure, which provides in-memory runtime information for virtual nodes.
// It includes methods for managing node state, tracking node runtime information, and retrieving node details.

// RuntimeInfoStore provide the in memory runtime information.
type RuntimeInfoStore struct {
	sync.RWMutex
	nodeIDToVNode map[string]*vnode.VNode // A map from node ID to VNode
}

// NewRuntimeInfoStore creates a new instance of RuntimeInfoStore.
func NewRuntimeInfoStore() *RuntimeInfoStore {
	return &RuntimeInfoStore{
		RWMutex:       sync.RWMutex{},
		nodeIDToVNode: make(map[string]*vnode.VNode),
	}
}

// NodeHeartbeatFromProviderArrived updates the latest message time for a given node ID.
func (r *RuntimeInfoStore) NodeHeartbeatFromProviderArrived(nodeID string) {
	r.Lock()
	defer r.Unlock()

	if vNode, has := r.nodeIDToVNode[nodeID]; has {
		vNode.Liveness.UpdateHeartBeatTime()
	}
}

// NodeShutdown removes a node from the running node map.
func (r *RuntimeInfoStore) NodeShutdown(nodeID string) {
	r.Lock()
	defer r.Unlock()

	if vNode, has := r.nodeIDToVNode[nodeID]; has {
		vNode.Liveness.Close()
		vNode.Shutdown()
	}
}

func (r *RuntimeInfoStore) GetRunningVNodes() []*vnode.VNode {
	r.Lock()
	defer r.Unlock()
	runningVNodes := make([]*vnode.VNode, 0)
	for _, vNode := range r.nodeIDToVNode {
		if vNode.Liveness.IsAlive() {
			runningVNodes = append(runningVNodes, vNode)
		}
	}
	return runningVNodes
}

// RunningNodeNum returns the number of running nodes.
func (r *RuntimeInfoStore) RunningNodeNum() int {
	r.Lock()
	defer r.Unlock()

	return len(r.GetRunningVNodes())
}

// AllNodeNum returns the number of all nodes.
func (r *RuntimeInfoStore) AllNodeNum() int {
	r.Lock()
	defer r.Unlock()

	return len(r.nodeIDToVNode)
}

// AddVNode adds a VNode to the node ID to VNode map.
func (r *RuntimeInfoStore) AddVNode(nodeID string, vnode *vnode.VNode) {
	r.Lock()
	defer r.Unlock()

	r.nodeIDToVNode[nodeID] = vnode
}

// DeleteVNode removes a VNode from the node ID to VNode map.
func (r *RuntimeInfoStore) DeleteVNode(nodeID string) {
	r.Lock()
	defer r.Unlock()

	delete(r.nodeIDToVNode, nodeID)
}

// GetVNode returns a VNode for a given node ID.
func (r *RuntimeInfoStore) GetVNode(nodeID string) *vnode.VNode {
	r.RLock()
	defer r.RUnlock()
	return r.nodeIDToVNode[nodeID]
}

// GetVNodes returns all VNodes.
func (r *RuntimeInfoStore) GetVNodes() []*vnode.VNode {
	r.RLock()
	defer r.RUnlock()
	ret := make([]*vnode.VNode, 0)
	for _, kn := range r.nodeIDToVNode {
		ret = append(ret, kn)
	}
	return ret
}

// GetVNodeByNodeName returns a VNode for a given node name.
func (r *RuntimeInfoStore) GetVNodeByNodeName(nodeName string) *vnode.VNode {
	r.Lock()
	defer r.Unlock()
	nodeID := utils.ExtractNodeIDFromNodeName(nodeName)
	return r.nodeIDToVNode[nodeID]
}

// GetLeaseOutdatedVNodeIDs returns the node ids of outdated leases.
func (r *RuntimeInfoStore) GetLeaseOutdatedVNodeIDs(clientID string) []string {
	r.Lock()
	defer r.Unlock()
	ret := make([]string, 0)
	for nodeId, vNode := range r.nodeIDToVNode {
		if !vNode.IsLeader(clientID) {
			ret = append(ret, nodeId)
		}
	}
	return ret
}

func (r *RuntimeInfoStore) GetLeaseOccupiedVNodeIDs(clientID string) []string {
	r.Lock()
	defer r.Unlock()
	ret := make([]string, 0)
	for nodeId, vNode := range r.nodeIDToVNode {
		if vNode.IsLeader(clientID) {
			ret = append(ret, nodeId)
		}
	}
	return ret
}

func (r *RuntimeInfoStore) GetLeaseOccupiedVNodes(clientID string) []*vnode.VNode {
	r.Lock()
	defer r.Unlock()
	ret := make([]*vnode.VNode, 0)
	for _, vNode := range r.nodeIDToVNode {
		if vNode.IsLeader(clientID) {
			ret = append(ret, vNode)
		}
	}
	return ret
}

// GetUnReachableNodeIds returns the node information of nodes that are not reachable.
func (r *RuntimeInfoStore) GetUnReachableNodeIds() []string {
	r.Lock()
	defer r.Unlock()
	ret := make([]string, 0)

	for nodeId, vNode := range r.nodeIDToVNode {
		if !vNode.Liveness.IsAlive() {
			ret = append(ret, nodeId)
		}
	}
	return ret
}
