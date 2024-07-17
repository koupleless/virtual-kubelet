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

package controller

import (
	"github.com/koupleless/virtual-kubelet/java/pod/node"
	"github.com/pkg/errors"
	"sync"
	"time"
)

// RuntimeInfoStore provide the in memory runtime information.
type RuntimeInfoStore struct {
	sync.RWMutex
	baseIDToKouplelessNode map[string]*node.KouplelessNode
	baseIDLock             map[string]bool
	baseLatestMsgTime      map[string]int64
}

func NewRuntimeInfoStore() *RuntimeInfoStore {
	return &RuntimeInfoStore{
		RWMutex:                sync.RWMutex{},
		baseIDToKouplelessNode: make(map[string]*node.KouplelessNode),
		baseLatestMsgTime:      make(map[string]int64),
		baseIDLock:             make(map[string]bool),
	}
}

func (r *RuntimeInfoStore) PutKouplelessNode(baseID string, k *node.KouplelessNode) {
	r.Lock()
	defer r.Unlock()

	r.baseIDToKouplelessNode[baseID] = k
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

func (r *RuntimeInfoStore) DeleteKouplelessNode(baseID string) {
	r.Lock()
	defer r.Unlock()

	delete(r.baseIDToKouplelessNode, baseID)
	delete(r.baseLatestMsgTime, baseID)
	delete(r.baseIDLock, baseID)
}

func (r *RuntimeInfoStore) GetKouplelessNode(baseID string) *node.KouplelessNode {
	r.RLock()
	defer r.RUnlock()
	return r.baseIDToKouplelessNode[baseID]
}

func (r *RuntimeInfoStore) GetKouplelessNodes() []*node.KouplelessNode {
	r.RLock()
	defer r.RUnlock()
	ret := make([]*node.KouplelessNode, 0)
	for _, kn := range r.baseIDToKouplelessNode {
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
