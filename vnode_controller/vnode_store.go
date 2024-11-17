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
	"github.com/koupleless/virtual-kubelet/provider"
	"github.com/pkg/errors"
	"sync"
)

// Summary:
// This file defines the VNodeStore structure, which provides in-memory runtime information for virtual nodes.
// It includes methods for managing node state, tracking node runtime information, and retrieving node details.

// VNodeStore provide the in memory runtime information.
type VNodeStore struct {
	sync.RWMutex
	nodeIDToVNode map[string]*provider.VNode // A map from node ID to VNode
}

// NewVNodeStore creates a new instance of VNodeStore.
func NewVNodeStore() *VNodeStore {
	return &VNodeStore{
		RWMutex:       sync.RWMutex{},
		nodeIDToVNode: make(map[string]*provider.VNode),
	}
}

// NodeHeartbeatFromProviderArrived updates the latest message time for a given node ID.
func (r *VNodeStore) NodeHeartbeatFromProviderArrived(nodeID string) {
	r.Lock()
	defer r.Unlock()

	if vNode, has := r.nodeIDToVNode[nodeID]; has {
		vNode.Liveness.UpdateHeartBeatTime()
	}
}

// NodeShutdown removes a node from the running node map.
func (r *VNodeStore) NodeShutdown(nodeID string) {
	r.Lock()
	defer r.Unlock()

	if vNode, has := r.nodeIDToVNode[nodeID]; has {
		vNode.Liveness.Close()
		vNode.Shutdown()
	}
}

func (r *VNodeStore) GetRunningVNodes() []*provider.VNode {
	r.Lock()
	defer r.Unlock()
	runningVNodes := make([]*provider.VNode, 0)
	for _, vNode := range r.nodeIDToVNode {
		if vNode.Liveness.IsAlive() {
			runningVNodes = append(runningVNodes, vNode)
		}
	}
	return runningVNodes
}

// RunningNodeNum returns the number of running nodes.
func (r *VNodeStore) RunningNodeNum() int {
	r.Lock()
	defer r.Unlock()

	return len(r.GetRunningVNodes())
}

// AllNodeNum returns the number of all nodes.
func (r *VNodeStore) AllNodeNum() int {
	r.Lock()
	defer r.Unlock()

	return len(r.nodeIDToVNode)
}

// AddVNode adds a VNode to the node ID to VNode map.
func (r *VNodeStore) AddVNode(nodeID string, vnode *provider.VNode) error {
	r.Lock()
	defer r.Unlock()

	if _, has := r.nodeIDToVNode[nodeID]; has {
		return errors.Errorf("nodeID %s already exists", nodeID)
	}
	r.nodeIDToVNode[nodeID] = vnode
	return nil
}

// DeleteVNode removes a VNode from the node ID to VNode map.
func (r *VNodeStore) DeleteVNode(nodeID string) {
	r.Lock()
	defer r.Unlock()

	delete(r.nodeIDToVNode, nodeID)
}

// GetVNode returns a VNode for a given node ID.
func (r *VNodeStore) GetVNode(nodeID string) *provider.VNode {
	r.RLock()
	defer r.RUnlock()
	return r.nodeIDToVNode[nodeID]
}

// GetVNodes returns all VNodes.
func (r *VNodeStore) GetVNodes() []*provider.VNode {
	r.RLock()
	defer r.RUnlock()
	ret := make([]*provider.VNode, 0)
	for _, kn := range r.nodeIDToVNode {
		ret = append(ret, kn)
	}
	return ret
}

// GetVNodeByNodeName returns a VNode for a given node name.
func (r *VNodeStore) GetVNodeByNodeName(nodeName string) *provider.VNode {
	r.Lock()
	defer r.Unlock()
	nodeID := utils.ExtractNodeIDFromNodeName(nodeName)
	return r.nodeIDToVNode[nodeID]
}

// GetLeaseOutdatedVNodeIDs returns the node ids of outdated leases.
func (r *VNodeStore) GetLeaseOutdatedVNodeIDs(clientID string) []string {
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

func (r *VNodeStore) GetLeaseOccupiedVNodeIDs(clientID string) []string {
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

func (r *VNodeStore) GetLeaseOccupiedVNodes(clientID string) []*provider.VNode {
	r.Lock()
	defer r.Unlock()
	ret := make([]*provider.VNode, 0)
	for _, vNode := range r.nodeIDToVNode {
		if vNode.IsLeader(clientID) {
			ret = append(ret, vNode)
		}
	}
	return ret
}

// GetUnReachableVNodes returns the node information of nodes that are not reachable.
func (r *VNodeStore) GetUnReachableVNodes() []*provider.VNode {
	r.Lock()
	defer r.Unlock()
	ret := make([]*provider.VNode, 0)

	for _, vNode := range r.nodeIDToVNode {
		if !vNode.Liveness.IsAlive() {
			ret = append(ret, vNode)
		}
	}
	return ret
}
