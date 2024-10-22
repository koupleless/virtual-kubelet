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

package pod_provider

import (
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"sync"

	corev1 "k8s.io/api/core/v1"
)

// RuntimeInfoStore provide the in memory runtime information.
type RuntimeInfoStore struct {
	sync.RWMutex

	podKeyToPod                          map[string]*corev1.Pod
	containerUniqueKeyKeyToRelatedPodKey map[string]map[string]bool
	containerKeyToContainer              map[string]*corev1.Container

	latestContainerInfosFromNode map[string]*model.ContainerStatusData
}

func NewRuntimeInfoStore() *RuntimeInfoStore {
	return &RuntimeInfoStore{
		RWMutex:                              sync.RWMutex{},
		podKeyToPod:                          make(map[string]*corev1.Pod),
		containerUniqueKeyKeyToRelatedPodKey: make(map[string]map[string]bool),
		containerKeyToContainer:              make(map[string]*corev1.Container),
		latestContainerInfosFromNode:         make(map[string]*model.ContainerStatusData),
	}
}

func (r *RuntimeInfoStore) PutPod(pod *corev1.Pod, t tunnel.Tunnel) {
	r.Lock()
	defer r.Unlock()

	podKey := utils.GetPodKey(pod)

	// create or update
	r.podKeyToPod[podKey] = pod
	for _, container := range pod.Spec.Containers {
		containerKey := utils.GetContainerKey(podKey, container.Name)
		containerUniqueKey := t.GetContainerUniqueKey(podKey, &container)
		relatedPodKeyMap, has := r.containerUniqueKeyKeyToRelatedPodKey[containerUniqueKey]
		if !has {
			relatedPodKeyMap = make(map[string]bool)
		}
		relatedPodKeyMap[podKey] = true
		r.containerUniqueKeyKeyToRelatedPodKey[containerUniqueKey] = relatedPodKeyMap
		r.containerKeyToContainer[containerKey] = &container
	}
}

func (r *RuntimeInfoStore) DeletePod(podKey string, t tunnel.Tunnel) {
	r.Lock()
	defer r.Unlock()

	pod, has := r.podKeyToPod[podKey]
	delete(r.podKeyToPod, podKey)
	if has {
		for _, container := range pod.Spec.Containers {
			containerKey := utils.GetContainerKey(podKey, container.Name)
			containerUniqueKey := t.GetContainerUniqueKey(podKey, &container)
			relatedPodKeyMap, has := r.containerUniqueKeyKeyToRelatedPodKey[containerUniqueKey]
			if has {
				delete(relatedPodKeyMap, podKey)
			}
			delete(r.containerKeyToContainer, containerKey)
			r.containerUniqueKeyKeyToRelatedPodKey[containerUniqueKey] = relatedPodKeyMap
		}
	}
}

func (r *RuntimeInfoStore) GetRelatedPodKeysByContainerKey(containerKey string) []string {
	r.Lock()
	defer r.Unlock()
	relatedPodKeyMap := r.containerUniqueKeyKeyToRelatedPodKey[containerKey]
	ret := make([]string, 0)
	for podKey := range relatedPodKeyMap {
		ret = append(ret, podKey)
	}
	return ret
}

func (r *RuntimeInfoStore) GetContainer(containerKey string) *corev1.Container {
	r.RLock()
	defer r.RUnlock()
	return r.containerKeyToContainer[containerKey]
}

func (r *RuntimeInfoStore) GetPodByKey(podKey string) *corev1.Pod {
	r.RLock()
	defer r.RUnlock()
	return r.podKeyToPod[podKey]
}

func (r *RuntimeInfoStore) GetPods() []*corev1.Pod {
	r.RLock()
	defer r.RUnlock()

	ret := make([]*corev1.Pod, 0, len(r.podKeyToPod))
	for _, pod := range r.podKeyToPod {
		ret = append(ret, pod)
	}
	return ret
}

func (r *RuntimeInfoStore) PutContainerStatus(containerInfo model.ContainerStatusData) (updated bool) {
	r.Lock()
	defer r.Unlock()

	oldData, has := r.latestContainerInfosFromNode[containerInfo.Key]
	// check change time valid
	if has && oldData != nil {
		if !oldData.ChangeTime.Before(containerInfo.ChangeTime) {
			// old message, not process
			return
		}
		if !utils.IsContainerStatusDataEqual(oldData, &containerInfo) {
			updated = true
		}
	} else {
		updated = true
	}
	r.latestContainerInfosFromNode[containerInfo.Key] = &containerInfo
	return
}

func (r *RuntimeInfoStore) ClearContainerStatus(containerKey string) {
	r.Lock()
	defer r.Unlock()

	delete(r.latestContainerInfosFromNode, containerKey)
}

func (r *RuntimeInfoStore) GetLatestContainerInfos() []*model.ContainerStatusData {
	r.Lock()
	defer r.Unlock()
	containerInfos := make([]*model.ContainerStatusData, 0)
	for _, containerInfo := range r.latestContainerInfosFromNode {
		containerInfos = append(containerInfos, containerInfo)
	}
	return containerInfos
}

func (r *RuntimeInfoStore) GetLatestContainerInfoByContainerKey(containerKey string) *model.ContainerStatusData {
	r.Lock()
	defer r.Unlock()
	return r.latestContainerInfosFromNode[containerKey]
}
