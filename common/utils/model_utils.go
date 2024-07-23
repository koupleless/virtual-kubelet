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

package utils

import (
	"context"
	"github.com/google/go-cmp/cmp"
	"github.com/koupleless/arkctl/common/fileutil"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"time"
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

func (c ModelUtils) TranslateArkBizInfoToV1ContainerStatus(bizModel *ark.BizModel, bizInfo *ark.ArkBizInfo) *corev1.ContainerStatus {
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
		ret.State.Waiting = &corev1.ContainerStateWaiting{
			Reason:  "BizPending",
			Message: "Biz is waiting for installing",
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
		latestActivatedTime := time.UnixMilli(0)
		for _, record := range bizInfo.BizStateRecords {
			if record.State != "ACTIVATED" {
				continue
			}
			if len(record.ChangeTime) < 3 {
				continue
			}
			changeTime, err := time.Parse("2006-01-02 15:04:05", record.ChangeTime[:len(record.ChangeTime)-3])
			if err != nil {
				log.G(context.Background()).Errorf("failed to parse change time %s", record.ChangeTime)
				continue
			}
			if changeTime.UnixMilli() > latestActivatedTime.UnixMilli() {
				latestActivatedTime = changeTime
			}
		}
		ret.State.Running = &corev1.ContainerStateRunning{
			// for now we can just leave it empty,
			// in the future when the arklet supports this, we can fill this field.
			StartedAt: metav1.Time{
				Time: latestActivatedTime,
			},
		}
	}

	if bizInfo.BizState == "DEACTIVATED" {
		latestDeactivatedTime := time.UnixMilli(0)
		for _, record := range bizInfo.BizStateRecords {
			if record.State != "DEACTIVATED" {
				continue
			}
			if len(record.ChangeTime) < 3 {
				continue
			}
			changeTime, err := time.Parse("2006-01-02 15:04:05", record.ChangeTime[:len(record.ChangeTime)-3])
			if err != nil {
				log.G(context.Background()).Errorf("failed to parse change time %s", record.ChangeTime)
				continue
			}
			if changeTime.UnixMilli() > latestDeactivatedTime.UnixMilli() {
				latestDeactivatedTime = changeTime
			}
		}
		ret.State.Terminated = &corev1.ContainerStateTerminated{
			ExitCode: 1,
			Reason:   "BizDeactivated",
			Message:  "Biz is deactivated",
			FinishedAt: metav1.Time{
				Time: latestDeactivatedTime,
			},
			ContainerID: c.GetBizIdentityFromBizModel(bizModel),
		}
	}
	return ret
}

func (c ModelUtils) BuildVirtualNode(config *model.BuildVirtualNodeConfig, node *corev1.Node) {
	if node.ObjectMeta.Labels == nil {
		node.ObjectMeta.Labels = make(map[string]string)
	}
	node.Labels["base.koupleless.io/stack"] = config.TechStack
	node.Labels["base.koupleless.io/version"] = config.Version
	node.Labels["base.koupleless.io/name"] = config.BizName
	node.Spec.Taints = []corev1.Taint{
		{
			Key:    "schedule.koupleless.io/virtual-node",
			Value:  "True",
			Effect: corev1.TaintEffectNoExecute,
		},
	}
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
			corev1.ResourcePods: resource.MustParse("2000"),
		},
		Allocatable: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourcePods: resource.MustParse("2000"),
		},
	}
}

// PodsEqual checks if two pods are equal according to the fields we know that are allowed
// to be modified after startup time.
func PodsEqual(pod1, pod2 *corev1.Pod) bool {
	// Pod Update Only Permits update of:
	// - `spec.containers[*].image`
	// - `spec.initContainers[*].image`
	// - `spec.activeDeadlineSeconds`
	// - `spec.tolerations` (only additions to existing tolerations)
	// - `objectmeta.labels`
	// - `objectmeta.annotations`
	// compare the values of the pods to see if the values actually changed

	return cmp.Equal(pod1.Spec.Containers, pod2.Spec.Containers) &&
		cmp.Equal(pod1.Spec.InitContainers, pod2.Spec.InitContainers) &&
		cmp.Equal(pod1.Spec.ActiveDeadlineSeconds, pod2.Spec.ActiveDeadlineSeconds) &&
		cmp.Equal(pod1.Spec.Tolerations, pod2.Spec.Tolerations) &&
		cmp.Equal(pod1.ObjectMeta.Labels, pod2.Labels) &&
		cmp.Equal(pod1.ObjectMeta.Annotations, pod2.Annotations)

}
