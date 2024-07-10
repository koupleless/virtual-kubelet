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
	"fmt"
	"github.com/koupleless/virtual-kubelet/java/pod/node"
	"sync"
	"time"
)

// RuntimeInfoStore provide the in memory runtime information.
type RuntimeInfoStore struct {
	sync.RWMutex
	deviceIDToKouplelessNode map[string]*node.KouplelessNode
	deviceLatestMsgTime      map[string]int64
}

func NewRuntimeInfoStore() *RuntimeInfoStore {
	return &RuntimeInfoStore{
		RWMutex:                  sync.RWMutex{},
		deviceIDToKouplelessNode: make(map[string]*node.KouplelessNode),
		deviceLatestMsgTime:      make(map[string]int64),
	}
}

func (r *RuntimeInfoStore) PutKouplelessNode(deviceID string, k *node.KouplelessNode) {
	r.Lock()
	defer r.Unlock()

	r.deviceIDToKouplelessNode[deviceID] = k
}

func (r *RuntimeInfoStore) PutKouplelessNodeNX(deviceID string, k *node.KouplelessNode) error {
	r.Lock()
	defer r.Unlock()

	_, has := r.deviceIDToKouplelessNode[deviceID]
	if has {
		return fmt.Errorf("deviceID %s already exists", deviceID)
	}
	r.deviceIDToKouplelessNode[deviceID] = k
	return nil
}

func (r *RuntimeInfoStore) DeleteKouplelessNode(deviceID string) {
	r.Lock()
	defer r.Unlock()

	delete(r.deviceIDToKouplelessNode, deviceID)
	delete(r.deviceLatestMsgTime, deviceID)
}

func (r *RuntimeInfoStore) GetKouplelessNode(deviceID string) *node.KouplelessNode {
	r.RLock()
	defer r.RUnlock()
	return r.deviceIDToKouplelessNode[deviceID]
}

func (r *RuntimeInfoStore) GetKouplelessNodes() []*node.KouplelessNode {
	r.RLock()
	defer r.RUnlock()
	ret := make([]*node.KouplelessNode, 0)
	for _, node := range r.deviceIDToKouplelessNode {
		ret = append(ret, node)
	}
	return ret
}

func (r *RuntimeInfoStore) DeviceMsgArrived(deviceID string) {
	r.Lock()
	defer r.Unlock()
	r.deviceLatestMsgTime[deviceID] = time.Now().UnixMilli()
}

func (r *RuntimeInfoStore) GetOfflineDevices(maxUnreachableMilliSec int64) []string {
	r.Lock()
	defer r.Unlock()
	offlineDeviceIDs := make([]string, 0)
	minMsgTime := time.Now().UnixMilli() - maxUnreachableMilliSec
	for deviceID, latestMsgTime := range r.deviceLatestMsgTime {
		if latestMsgTime >= minMsgTime {
			continue
		}
		offlineDeviceIDs = append(offlineDeviceIDs, deviceID)
	}
	return offlineDeviceIDs
}
