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

package base_register_controller

import (
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/vnode/base_node"
	"github.com/pkg/errors"
	"strings"
	"sync"
	"time"
)

// RuntimeInfoStore provide the in memory runtime information.
type RuntimeInfoStore struct {
	sync.RWMutex
	baseIDToBaseNode  map[string]*base_node.BaseNode
	baseIDLock        map[string]bool
	baseLatestMsgTime map[string]int64
}

func NewRuntimeInfoStore() *RuntimeInfoStore {
	return &RuntimeInfoStore{
		RWMutex:           sync.RWMutex{},
		baseIDToBaseNode:  make(map[string]*base_node.BaseNode),
		baseLatestMsgTime: make(map[string]int64),
		baseIDLock:        make(map[string]bool),
	}
}

func (r *RuntimeInfoStore) PutBaseNode(baseID string, k *base_node.BaseNode) {
	r.Lock()
	defer r.Unlock()

	r.baseIDToBaseNode[baseID] = k
}

func (r *RuntimeInfoStore) PutBaseIDNX(baseID string) error {
	r.Lock()
	defer r.Unlock()

	if r.baseIDLock[baseID] {
		return errors.Errorf("baseID %s already exists", baseID)
	}
	r.baseIDLock[baseID] = true
	return nil
}

func (r *RuntimeInfoStore) DeleteBaseNode(baseID string) {
	r.Lock()
	defer r.Unlock()

	delete(r.baseIDToBaseNode, baseID)
	delete(r.baseLatestMsgTime, baseID)
	delete(r.baseIDLock, baseID)
}

func (r *RuntimeInfoStore) GetBaseNode(baseID string) *base_node.BaseNode {
	r.RLock()
	defer r.RUnlock()
	return r.baseIDToBaseNode[baseID]
}

func (r *RuntimeInfoStore) GetBaseNodes() []*base_node.BaseNode {
	r.RLock()
	defer r.RUnlock()
	ret := make([]*base_node.BaseNode, 0)
	for _, kn := range r.baseIDToBaseNode {
		ret = append(ret, kn)
	}
	return ret
}

func (r *RuntimeInfoStore) BaseMsgArrived(baseID string) {
	r.Lock()
	defer r.Unlock()
	r.baseLatestMsgTime[baseID] = time.Now().UnixMilli()
}

func (r *RuntimeInfoStore) GetOfflineBases(maxUnreachableMilliSec int64) []string {
	r.Lock()
	defer r.Unlock()
	offlineBaseIDs := make([]string, 0)
	minMsgTime := time.Now().UnixMilli() - maxUnreachableMilliSec
	for baseID, latestMsgTime := range r.baseLatestMsgTime {
		if latestMsgTime >= minMsgTime {
			continue
		}
		offlineBaseIDs = append(offlineBaseIDs, baseID)
	}
	return offlineBaseIDs
}

func (r *RuntimeInfoStore) getBaseIDFromNodeID(nodeID string) string {
	if !strings.HasPrefix(nodeID, model.VIRTUAL_NODE_NAME_PREFIX) {
		return ""
	}
	return nodeID[len(model.VIRTUAL_NODE_NAME_PREFIX):]
}

func (r *RuntimeInfoStore) GetBaseNodeByNodeID(nodeID string) *base_node.BaseNode {
	r.Lock()
	defer r.Unlock()
	baseID := r.getBaseIDFromNodeID(nodeID)
	return r.baseIDToBaseNode[baseID]
}
