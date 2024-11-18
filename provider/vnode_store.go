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

package provider

import (
	"github.com/pkg/errors"
	"sync"
)

// Summary:
// This file defines the VNodeStore structure, which provides in-memory runtime information for virtual nodes.
// It includes methods for managing node state, tracking node runtime information, and retrieving node details.

// VNodeStore provide the in memory runtime information.
type VNodeStore struct {
	sync.RWMutex
	nodeNameToVNode map[string]*VNode // A map from node ID to VNode
}

// NewVNodeStore creates a new instance of VNodeStore.
func NewVNodeStore() *VNodeStore {
	return &VNodeStore{
		RWMutex:         sync.RWMutex{},
		nodeNameToVNode: make(map[string]*VNode),
	}
}

// NodeHeartbeatFromProviderArrived updates the latest message time for a given node ID.
func (r *VNodeStore) NodeHeartbeatFromProviderArrived(nodeName string) {
	r.Lock()
	defer r.Unlock()

	if vNode, has := r.nodeNameToVNode[nodeName]; has {
		vNode.Liveness.UpdateHeartBeatTime()
	}
}

// NodeShutdown removes a node from the running node map.
func (r *VNodeStore) NodeShutdown(nodeName string) {
	r.Lock()
	defer r.Unlock()

	if vNode, has := r.nodeNameToVNode[nodeName]; has {
		vNode.Liveness.Close()
		vNode.Shutdown()
	}
}

// RunningNodeNum returns the number of running nodes.
func (r *VNodeStore) RunningNodeNum() int {
	r.Lock()
	defer r.Unlock()

	runningVNodes := make([]*VNode, 0)
	for _, vNode := range r.nodeNameToVNode {
		if vNode.Liveness.IsAlive() {
			runningVNodes = append(runningVNodes, vNode)
		}
	}

	return len(runningVNodes)
}

// AllNodeNum returns the number of all nodes.
func (r *VNodeStore) AllNodeNum() int {
	r.Lock()
	defer r.Unlock()

	return len(r.nodeNameToVNode)
}

// AddVNode adds a VNode to the node name to VNode map.
func (r *VNodeStore) AddVNode(nodeName string, vnode *VNode) error {
	r.Lock()
	defer r.Unlock()

	if _, has := r.nodeNameToVNode[nodeName]; has {
		return errors.Errorf("node %s already exists", nodeName)
	}
	r.nodeNameToVNode[nodeName] = vnode
	return nil
}

// DeleteVNode removes a VNode from the node name to VNode map.
func (r *VNodeStore) DeleteVNode(nodeName string) {
	r.Lock()
	defer r.Unlock()

	delete(r.nodeNameToVNode, nodeName)
}

// GetVNode returns a VNode for a given node ID.
func (r *VNodeStore) GetVNode(nodeName string) *VNode {
	r.RLock()
	defer r.RUnlock()
	return r.nodeNameToVNode[nodeName]
}

// GetVNodes returns all VNodes.
func (r *VNodeStore) GetVNodes() []*VNode {
	r.RLock()
	defer r.RUnlock()
	ret := make([]*VNode, 0)
	for _, kn := range r.nodeNameToVNode {
		ret = append(ret, kn)
	}
	return ret
}

// GetVNodeByNodeName returns a VNode for a given node name.
func (r *VNodeStore) GetVNodeByNodeName(nodeName string) *VNode {
	r.Lock()
	defer r.Unlock()
	return r.nodeNameToVNode[nodeName]
}

// GetLeaseOutdatedVNodeNames returns the node ids of outdated leases.
func (r *VNodeStore) GetLeaseOutdatedVNodeNames(clientID string) []string {
	r.Lock()
	defer r.Unlock()
	ret := make([]string, 0)
	for nodeName, vNode := range r.nodeNameToVNode {
		if !vNode.IsLeader(clientID) {
			ret = append(ret, nodeName)
		}
	}
	return ret
}

func (r *VNodeStore) GetLeaseOccupiedVNodes(clientID string) []*VNode {
	r.Lock()
	defer r.Unlock()
	ret := make([]*VNode, 0)
	for _, vNode := range r.nodeNameToVNode {
		if vNode.IsLeader(clientID) {
			ret = append(ret, vNode)
		}
	}
	return ret
}

// GetUnReachableVNodes returns the node information of nodes that are not reachable.
func (r *VNodeStore) GetUnReachableVNodes() []*VNode {
	r.Lock()
	defer r.Unlock()
	ret := make([]*VNode, 0)

	for _, vNode := range r.nodeNameToVNode {
		if !vNode.Liveness.IsAlive() {
			ret = append(ret, vNode)
		}
	}
	return ret
}
