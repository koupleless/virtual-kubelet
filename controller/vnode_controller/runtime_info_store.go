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
	"sync"
	"time"

	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/vnode"
	"github.com/pkg/errors"
)

// Summary:
// This file defines the RuntimeInfoStore structure, which provides in-memory runtime information for virtual nodes.
// It includes methods for managing node state, tracking node runtime information, and retrieving node details.

// RuntimeInfoStore provide the in memory runtime information.
type RuntimeInfoStore struct {
	sync.RWMutex
	nodeIDToVNode   map[string]*vnode.VNode // A map from node ID to VNode
	startLock       map[string]bool         // A map to lock the start of a node
	runningNodeMap  map[string]bool         // A map to track running nodes
	allNodeMap      map[string]bool         // A map to track all nodes
	leaseUpdateTime map[string]time.Time    // A map to track the latest lease update time
	latestMsgTime   map[string]time.Time    // A map to track the latest message time
}

// NewRuntimeInfoStore creates a new instance of RuntimeInfoStore.
func NewRuntimeInfoStore() *RuntimeInfoStore {
	return &RuntimeInfoStore{
		RWMutex:         sync.RWMutex{},
		nodeIDToVNode:   make(map[string]*vnode.VNode),
		startLock:       make(map[string]bool),
		runningNodeMap:  make(map[string]bool),
		allNodeMap:      make(map[string]bool),
		leaseUpdateTime: make(map[string]time.Time),
		latestMsgTime:   make(map[string]time.Time),
	}
}

// NodeRunning updates the running node map and the latest message time for a given node ID.
func (r *RuntimeInfoStore) NodeRunning(nodeID string) {
	r.Lock()
	defer r.Unlock()

	r.runningNodeMap[nodeID] = true
	r.latestMsgTime[nodeID] = time.Now()
}

// NodeShutdown removes a node from the running node map.
func (r *RuntimeInfoStore) NodeShutdown(nodeID string) {
	r.Lock()
	defer r.Unlock()

	delete(r.runningNodeMap, nodeID)
}

// RunningNodeNum returns the number of running nodes.
func (r *RuntimeInfoStore) RunningNodeNum() int {
	r.Lock()
	defer r.Unlock()

	return len(r.runningNodeMap)
}

// PutNode adds a node to the all node map.
func (r *RuntimeInfoStore) PutNode(nodeName string) {
	r.Lock()
	defer r.Unlock()

	r.allNodeMap[nodeName] = true
}

// DeleteNode removes a node from the all node map.
func (r *RuntimeInfoStore) DeleteNode(nodeName string) {
	r.Lock()
	defer r.Unlock()

	delete(r.allNodeMap, nodeName)
}

// AllNodeNum returns the number of all nodes.
func (r *RuntimeInfoStore) AllNodeNum() int {
	r.Lock()
	defer r.Unlock()

	return len(r.allNodeMap)
}

// PutVNode adds a VNode to the node ID to VNode map.
func (r *RuntimeInfoStore) PutVNode(nodeID string, vnode *vnode.VNode) {
	r.Lock()
	defer r.Unlock()

	r.nodeIDToVNode[nodeID] = vnode
}

// PutVNodeIDNX locks the start of a node.
func (r *RuntimeInfoStore) PutVNodeIDNX(nodeID string) error {
	r.Lock()
	defer r.Unlock()

	if r.startLock[nodeID] {
		return errors.Errorf("nodeID %s already exists", nodeID)
	}
	r.startLock[nodeID] = true
	return nil
}

// DeleteVNode removes a VNode from the node ID to VNode map.
func (r *RuntimeInfoStore) DeleteVNode(nodeID string) {
	r.Lock()
	defer r.Unlock()

	delete(r.nodeIDToVNode, nodeID)
	delete(r.startLock, nodeID)
	delete(r.latestMsgTime, nodeID)
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

// PutVNodeLeaseLatestUpdateTime updates the lease update time for a given node name.
func (r *RuntimeInfoStore) PutVNodeLeaseLatestUpdateTime(nodeName string, renewTime time.Time) {
	r.Lock()
	defer r.Unlock()
	r.leaseUpdateTime[nodeName] = renewTime
}

// GetLeaseOutdatedVNodeName returns the node names of outdated leases.
func (r *RuntimeInfoStore) GetLeaseOutdatedVNodeName(leaseDuration time.Duration) []string {
	r.Lock()
	defer r.Unlock()
	now := time.Now()
	ret := make([]string, 0)
	for nodeName, latestUpdateTime := range r.leaseUpdateTime {
		if now.Sub(latestUpdateTime) > leaseDuration {
			ret = append(ret, nodeName)
		}
	}
	return ret
}

// GetNotReachableNodeInfos returns the node information of nodes that are not reachable.
func (r *RuntimeInfoStore) GetNotReachableNodeInfos(maxUnreachableDuration time.Duration) []model.UnreachableNodeInfo {
	r.Lock()
	defer r.Unlock()
	now := time.Now()
	ret := make([]model.UnreachableNodeInfo, 0)
	for nodeID, latestUpdateTime := range r.latestMsgTime {
		if now.Sub(latestUpdateTime) > maxUnreachableDuration {
			ret = append(ret, model.UnreachableNodeInfo{
				NodeID:              nodeID,
				LatestReachableTime: latestUpdateTime,
			})
		}
	}
	return ret
}

// NodeMsgArrived updates the latest message time for a given node ID.
func (r *RuntimeInfoStore) NodeMsgArrived(nodeID string) {
	r.Lock()
	defer r.Unlock()

	r.latestMsgTime[nodeID] = time.Now()
}
