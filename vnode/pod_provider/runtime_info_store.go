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
	"strings"
	"sync"

	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"

	corev1 "k8s.io/api/core/v1"
)

// RuntimeInfoStore provides in-memory runtime information.
type RuntimeInfoStore struct {
	sync.RWMutex // This mutex is used for thread-safe access to the store.

	podKeyToPod                          map[string]*corev1.Pod       // Maps pod keys to their corresponding pods.
	containerUniqueKeyKeyToRelatedPodKey map[string]map[string]bool   // Maps container unique keys to their related pod keys.
	containerKeyToContainer              map[string]*corev1.Container // Maps container keys to their corresponding containers.
}

func NewRuntimeInfoStore() *RuntimeInfoStore {
	return &RuntimeInfoStore{
		RWMutex:                              sync.RWMutex{},
		podKeyToPod:                          make(map[string]*corev1.Pod),
		containerUniqueKeyKeyToRelatedPodKey: make(map[string]map[string]bool),
		containerKeyToContainer:              make(map[string]*corev1.Container),
	}
}

// PutPod function updates or adds a pod to the RuntimeInfoStore.
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

// DeletePod function removes a pod from the RuntimeInfoStore.
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

// GetRelatedPodKeysByContainerKey function retrieves a list of pod keys related to a given container key.
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

// GetContainer function retrieves a container by its key.
func (r *RuntimeInfoStore) GetContainer(containerKey string) *corev1.Container {
	r.RLock()
	defer r.RUnlock()
	return r.containerKeyToContainer[containerKey]
}

// GetPodByKey function retrieves a pod by its key.
func (r *RuntimeInfoStore) GetPodByKey(podKey string) *corev1.Pod {
	r.RLock()
	defer r.RUnlock()
	return r.podKeyToPod[podKey]
}

// GetPods function retrieves all pods in the RuntimeInfoStore.
func (r *RuntimeInfoStore) GetPods() []*corev1.Pod {
	r.RLock()
	defer r.RUnlock()

	ret := make([]*corev1.Pod, 0, len(r.podKeyToPod))
	for _, pod := range r.podKeyToPod {
		ret = append(ret, pod)
	}
	return ret
}

func (r *RuntimeInfoStore) CheckContainerStatusNeedSync(containerInfo model.ContainerStatusData) (needSync bool) {
	r.Lock()
	defer r.Unlock()

	var oldStatus *model.ContainerStatusData
	if pod, found := r.podKeyToPod[containerInfo.PodKey]; found {
		var matchedStatus *corev1.ContainerStatus
		var matchedContainer *corev1.Container
		for _, status := range pod.Status.ContainerStatuses {
			if (status.Name == containerInfo.Name) && strings.Contains(status.Image, ".jar") {
				matchedStatus = &status
			}
		}
		for _, container := range pod.Spec.Containers {
			if container.Name == containerInfo.Name {
				matchedContainer = &container
			}
		}

		if matchedStatus != nil && matchedContainer != nil {
			oldStatus = &model.ContainerStatusData{
				Key:        utils.GetContainerUniqueKey(matchedContainer),
				Name:       matchedStatus.Name,
				PodKey:     containerInfo.PodKey,
				State:      model.ContainerStateActivated,
				ChangeTime: matchedStatus.State.Running.StartedAt.Time,
			}
		}
	}

	// check change time valid
	if oldStatus != nil {
		if !oldStatus.ChangeTime.Before(containerInfo.ChangeTime) {
			// old message, not process
			return
		}
		if !utils.IsContainerStatusDataEqual(oldStatus, &containerInfo) {
			needSync = true
		}
	} else {
		needSync = true
	}
	return needSync
}
