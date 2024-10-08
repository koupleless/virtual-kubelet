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
	"github.com/pkg/errors"
	"sync"
	"time"
)

// RuntimeInfoStore provide the in memory runtime information.
type RuntimeInfoStore struct {
	sync.RWMutex
	nodeIDToVNode   map[string]*vnode.VNode
	startLock       map[string]bool
	runningNodeMap  map[string]bool
	allNodeMap      map[string]bool
	leaseUpdateTime map[string]time.Time
}

func NewRuntimeInfoStore() *RuntimeInfoStore {
	return &RuntimeInfoStore{
		RWMutex:         sync.RWMutex{},
		nodeIDToVNode:   make(map[string]*vnode.VNode),
		startLock:       make(map[string]bool),
		runningNodeMap:  make(map[string]bool),
		allNodeMap:      make(map[string]bool),
		leaseUpdateTime: make(map[string]time.Time),
	}
}

func (r *RuntimeInfoStore) NodeRunning(nodeName string) {
	r.Lock()
	defer r.Unlock()

	r.runningNodeMap[nodeName] = true
}

func (r *RuntimeInfoStore) NodeShutdown(nodeName string) {
	r.Lock()
	defer r.Unlock()

	delete(r.runningNodeMap, nodeName)
}

func (r *RuntimeInfoStore) RunningNodeNum() int {
	r.Lock()
	defer r.Unlock()

	return len(r.runningNodeMap)
}

func (r *RuntimeInfoStore) PutNode(nodeName string) {
	r.Lock()
	defer r.Unlock()

	r.allNodeMap[nodeName] = true
}

func (r *RuntimeInfoStore) DeleteNode(nodeName string) {
	r.Lock()
	defer r.Unlock()

	delete(r.allNodeMap, nodeName)
}

func (r *RuntimeInfoStore) AllNodeNum() int {
	r.Lock()
	defer r.Unlock()

	return len(r.allNodeMap)
}

func (r *RuntimeInfoStore) PutVNode(nodeID string, vnode *vnode.VNode) {
	r.Lock()
	defer r.Unlock()

	r.nodeIDToVNode[nodeID] = vnode
}

func (r *RuntimeInfoStore) PutVNodeIDNX(nodeID string) error {
	r.Lock()
	defer r.Unlock()

	if r.startLock[nodeID] {
		return errors.Errorf("nodeID %s already exists", nodeID)
	}
	r.startLock[nodeID] = true
	return nil
}

func (r *RuntimeInfoStore) DeleteVNode(nodeID string) {
	r.Lock()
	defer r.Unlock()

	delete(r.nodeIDToVNode, nodeID)
	delete(r.startLock, nodeID)
}

func (r *RuntimeInfoStore) GetVNode(nodeID string) *vnode.VNode {
	r.RLock()
	defer r.RUnlock()
	return r.nodeIDToVNode[nodeID]
}

func (r *RuntimeInfoStore) GetVNodes() []*vnode.VNode {
	r.RLock()
	defer r.RUnlock()
	ret := make([]*vnode.VNode, 0)
	for _, kn := range r.nodeIDToVNode {
		ret = append(ret, kn)
	}
	return ret
}

func (r *RuntimeInfoStore) GetVNodeByNodeName(nodeName string) *vnode.VNode {
	r.Lock()
	defer r.Unlock()
	nodeID := utils.ExtractNodeIDFromNodeName(nodeName)
	return r.nodeIDToVNode[nodeID]
}

func (r *RuntimeInfoStore) PutVNodeLeaseLatestUpdateTime(nodeName string, renewTime time.Time) {
	r.Lock()
	defer r.Unlock()
	r.leaseUpdateTime[nodeName] = renewTime
}

func (r *RuntimeInfoStore) GetLeaseOutdatedVNodeName(duration time.Duration) []string {
	r.Lock()
	defer r.Unlock()
	now := time.Now()
	ret := make([]string, 0)
	for nodeName, latestUpdateTime := range r.leaseUpdateTime {
		if now.Sub(latestUpdateTime) > duration {
			ret = append(ret, nodeName)
		}
	}
	return ret
}
