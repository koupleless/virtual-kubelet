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

	podKeyToPod map[string]*corev1.Pod // Maps pod keys to their corresponding pods.
}

func NewRuntimeInfoStore() *RuntimeInfoStore {
	return &RuntimeInfoStore{
		RWMutex:     sync.RWMutex{},
		podKeyToPod: make(map[string]*corev1.Pod),
	}
}

// PutPod function updates or adds a pod to the RuntimeInfoStore.
func (r *RuntimeInfoStore) PutPod(pod *corev1.Pod, t tunnel.Tunnel) {
	r.Lock()
	defer r.Unlock()

	podKey := utils.GetPodKey(pod)

	// create or update
	r.podKeyToPod[podKey] = pod
}

// DeletePod function removes a pod from the RuntimeInfoStore.
func (r *RuntimeInfoStore) DeletePod(podKey string, t tunnel.Tunnel) {
	r.Lock()
	defer r.Unlock()

	delete(r.podKeyToPod, podKey)
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

		if matchedContainer != nil {
			oldStatus = &model.ContainerStatusData{
				Key:    utils.GetContainerUniqueKey(matchedContainer),
				Name:   matchedContainer.Name,
				PodKey: containerInfo.PodKey,
			}
			if matchedStatus != nil {
				if matchedStatus.State.Running != nil {
					oldStatus.ChangeTime = matchedStatus.State.Running.StartedAt.Time
				}
				if matchedStatus.State.Terminated != nil {
					oldStatus.ChangeTime = matchedStatus.State.Terminated.FinishedAt.Time
				}
				oldStatus.State = utils.GetBizStateFromContainerState(*matchedStatus)
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
