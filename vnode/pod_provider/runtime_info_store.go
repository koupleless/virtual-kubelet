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
	"time"

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

func (r *RuntimeInfoStore) CheckContainerStatusNeedSync(bizStatusData model.BizStatusData) bool {
	r.Lock()
	defer r.Unlock()

	if pod, found := r.podKeyToPod[bizStatusData.PodKey]; found {
		var matchedStatus *corev1.ContainerStatus
		var matchedContainer *corev1.Container
		for _, status := range pod.Status.ContainerStatuses {
			if (status.Name == bizStatusData.Name) && strings.Contains(status.Image, ".jar") {
				matchedStatus = &status
			}
		}
		for _, container := range pod.Spec.Containers {
			if container.Name == bizStatusData.Name {
				matchedContainer = &container
			}
		}

		// the earliest change time of the container status when no time
		oldChangeTime := time.Time{}
		if matchedContainer != nil {
			if matchedStatus != nil {
				if matchedStatus.State.Running != nil {
					oldChangeTime = matchedStatus.State.Running.StartedAt.Time
				}
				if matchedStatus.State.Terminated != nil {
					oldChangeTime = matchedStatus.State.Terminated.FinishedAt.Time
				}
				if matchedStatus.State.Waiting != nil && pod.Status.Conditions != nil && len(pod.Status.Conditions) > 0 {
					oldChangeTime = pod.Status.Conditions[0].LastTransitionTime.Time
				}
			}
		}

		// TODO: 优化 bizStatusData.ChangeTime，只有 bizState 变化的时间才需要更新
		if bizStatusData.ChangeTime.After(oldChangeTime) {
			return true
		} else {
			return false
		}
	}

	// no pod found, no need to sync
	return false
}
