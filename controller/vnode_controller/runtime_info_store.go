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
	nodeIDToVNode      map[string]*vnode.VNode
	nodeIDLock         map[string]bool
	vnodeLatestMsgTime map[string]int64
}

func NewRuntimeInfoStore() *RuntimeInfoStore {
	return &RuntimeInfoStore{
		RWMutex:            sync.RWMutex{},
		nodeIDToVNode:      make(map[string]*vnode.VNode),
		vnodeLatestMsgTime: make(map[string]int64),
		nodeIDLock:         make(map[string]bool),
	}
}

func (r *RuntimeInfoStore) PutVNode(nodeID string, vnode *vnode.VNode) {
	r.Lock()
	defer r.Unlock()

	r.nodeIDToVNode[nodeID] = vnode
}

func (r *RuntimeInfoStore) PutVNodeIDNX(nodeID string) error {
	r.Lock()
	defer r.Unlock()

	if r.nodeIDLock[nodeID] {
		return errors.Errorf("nodeID %s already exists", nodeID)
	}
	r.nodeIDLock[nodeID] = true
	return nil
}

func (r *RuntimeInfoStore) DeleteVNode(nodeID string) {
	r.Lock()
	defer r.Unlock()

	delete(r.nodeIDToVNode, nodeID)
	delete(r.vnodeLatestMsgTime, nodeID)
	delete(r.nodeIDLock, nodeID)
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

func (r *RuntimeInfoStore) NodeMsgArrived(nodeID string) {
	r.Lock()
	defer r.Unlock()
	r.vnodeLatestMsgTime[nodeID] = time.Now().UnixMilli()
}

func (r *RuntimeInfoStore) GetOfflineNodes(maxUnreachableMilliSec int64) []string {
	r.Lock()
	defer r.Unlock()
	offlineNodeIDs := make([]string, 0)
	minMsgTime := time.Now().UnixMilli() - maxUnreachableMilliSec
	for nodeID, latestMsgTime := range r.vnodeLatestMsgTime {
		if latestMsgTime >= minMsgTime {
			continue
		}
		offlineNodeIDs = append(offlineNodeIDs, nodeID)
	}
	return offlineNodeIDs
}

func (r *RuntimeInfoStore) GetVNodeByNodeName(nodeName string) *vnode.VNode {
	r.Lock()
	defer r.Unlock()
	nodeID := utils.ExtractNodeIDFromNodeName(nodeName)
	return r.nodeIDToVNode[nodeID]
}
