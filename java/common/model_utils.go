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

package common

import (
	"context"
	"github.com/koupleless/arkctl/common/fileutil"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/java/model"
	"github.com/virtual-kubelet/virtual-kubelet/node/api/statsv1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"strconv"
)

// ModelUtils
// reference spec: https://github.com/koupleless/module-controller/discussions/8
// the corresponding implementation in the above spec.
type ModelUtils struct {
}

func (c ModelUtils) CmpBizModel(a, b *ark.BizModel) bool {
	return a.BizName == b.BizName && a.BizVersion == b.BizVersion
}

func (c ModelUtils) GetPodKey(pod *corev1.Pod) string {
	return pod.Namespace + "/" + pod.Name
}

func (c ModelUtils) GetBizIdentityFromBizModel(biz *ark.BizModel) string {
	return biz.BizName + ":" + biz.BizVersion
}

func (c ModelUtils) GetBizIdentityFromBizInfo(biz *ark.ArkBizInfo) string {
	return biz.BizName + ":" + biz.BizVersion
}

func (c ModelUtils) TranslateCoreV1ContainerToBizModel(container corev1.Container) ark.BizModel {
	bizVersion := ""
	for _, env := range container.Env {
		if env.Name == "BIZ_VERSION" {
			bizVersion = env.Value
			break
		}
	}

	return ark.BizModel{
		BizName:    container.Name,
		BizVersion: bizVersion,
		BizUrl:     fileutil.FileUrl(container.Image),
	}
}

func (c ModelUtils) GetBizModelsFromCoreV1Pod(pod *corev1.Pod) []*ark.BizModel {
	ret := make([]*ark.BizModel, len(pod.Spec.Containers))
	for i, container := range pod.Spec.Containers {
		bizModel := c.TranslateCoreV1ContainerToBizModel(container)
		ret[i] = &bizModel
	}
	return ret
}

func (c ModelUtils) TranslateArkBizInfoToV1ContainerStatus(bizModel *ark.BizModel, bizInfo *ark.ArkBizInfo, baseProcessRunning bool) *corev1.ContainerStatus {
	// todo: wait arklet support the timestamp return in bizInfo
	started :=
		bizInfo != nil && bizInfo.BizState == "ACTIVATED"

	ret := &corev1.ContainerStatus{
		Name:        bizModel.BizName,
		ContainerID: c.GetBizIdentityFromBizModel(bizModel),
		State:       corev1.ContainerState{},
		Ready:       started,
		Started:     &started,
		Image:       string(bizModel.BizUrl),
		ImageID:     string(bizModel.BizUrl),
	}

	if bizInfo == nil {
		// not installing
		if !baseProcessRunning {
			ret.State.Waiting = &corev1.ContainerStateWaiting{
				Reason:  "BaseDown",
				Message: "Base process down, waiting for restart",
			}
		} else {
			ret.State.Waiting = &corev1.ContainerStateWaiting{
				Reason:  "BizPending",
				Message: "Biz is waiting for installing",
			}
		}
		return ret
	}

	if bizInfo.BizState == "RESOLVED" {
		// installing
		ret.State.Waiting = &corev1.ContainerStateWaiting{
			Reason:  "BizResolved",
			Message: "Biz resolved",
		}
		return ret
	}

	// the module install progress is ultra fast, usually on takes seconds.
	// therefore, the operation method should all be performed in sync way.
	// and there would be no waiting state
	if bizInfo.BizState == "ACTIVATED" {
		ret.State.Running = &corev1.ContainerStateRunning{
			// for now we can just leave it empty,
			// in the future when the arklet supports this, we can fill this field.
			StartedAt: metav1.Time{},
		}
	}

	// todo: need arklet support get biz status of deactivated
	if bizInfo.BizState == "DEACTIVATED" {
		ret.State.Terminated = &corev1.ContainerStateTerminated{
			ExitCode:    1,
			Reason:      "BizDeactivated",
			Message:     "Biz is deactivated",
			FinishedAt:  metav1.Time{},
			StartedAt:   metav1.Time{},
			ContainerID: c.GetBizIdentityFromBizModel(bizModel),
		}
	}
	return ret
}

func (c ModelUtils) BuildVirtualNode(config *model.BuildVirtualNodeConfig, arkService ark.Service, node *corev1.Node) {
	if node.ObjectMeta.Labels == nil {
		node.ObjectMeta.Labels = make(map[string]string)
	}
	node.Labels["base.koupleless.io/stack"] = config.TechStack
	node.Labels["base.koupleless.io/version"] = config.Version
	node.Spec.Taints = []corev1.Taint{
		{
			Key:    "schedule.koupleless.io/virtual-node",
			Value:  "True",
			Effect: corev1.TaintEffectNoExecute,
		},
	}
	node.Status = corev1.NodeStatus{
		Phase: corev1.NodeRunning,
		Addresses: []corev1.NodeAddress{
			{
				Type:    corev1.NodeInternalIP,
				Address: config.NodeIP,
			},
		},
		Conditions: []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		},
		Capacity: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourcePods: resource.MustParse(strconv.Itoa(config.VPodCapacity)),
		},
	}
	// TODO add base process liveness check, now use query for check
	_, err := arkService.QueryAllBiz(context.Background(), ark.QueryAllArkBizRequest{
		HostName: model.LOOP_BACK_IP,
		Port:     model.ARK_SERVICE_PORT,
	})
	if err != nil {
		// base process not ready
		node.Status = corev1.NodeStatus{
			Phase: corev1.NodePending,
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: config.NodeIP,
				},
			},
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				},
			},
			Capacity: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourcePods: resource.MustParse(strconv.Itoa(config.VPodCapacity)),
			},
		}
	}
}

func (c ModelUtils) TranslatePodToSummaryPodStats(pod *corev1.Pod) statsv1alpha1.PodStats {
	// read curr vm stats
	containerStatsList := make([]statsv1alpha1.ContainerStats, len(pod.Spec.Containers))
	for index, container := range pod.Spec.Containers {
		containerStatsList[index] = c.TranslateContainerToSummaryContainerStats(&container)
	}
	return statsv1alpha1.PodStats{
		PodRef: statsv1alpha1.PodReference{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			UID:       string(pod.UID),
		},
		StartTime:  *pod.Status.StartTime,
		Containers: containerStatsList,
		CPU: &statsv1alpha1.CPUStats{
			Time:                 metav1.Time{},
			UsageNanoCores:       nil,
			UsageCoreNanoSeconds: nil,
		},
		Memory:           nil,
		Network:          nil,
		VolumeStats:      nil,
		EphemeralStorage: nil,
		ProcessStats:     nil,
	}
}

func (c ModelUtils) TranslateContainerToSummaryContainerStats(container *corev1.Container) statsv1alpha1.ContainerStats {
	// TODO implement later
	return statsv1alpha1.ContainerStats{}
}
