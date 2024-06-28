package common

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/java/model"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

var moduleUtils = ModelUtils{}

func TestModelUtils_BuildVirtualNode(t *testing.T) {
	node := &corev1.Node{
		TypeMeta:   metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{},
		Spec:       corev1.NodeSpec{},
		Status:     corev1.NodeStatus{},
	}
	arkService := ark.BuildService(context.Background())
	moduleUtils.BuildVirtualNode(&model.BuildVirtualNodeConfig{
		NodeIP:       "127.0.0.1",
		TechStack:    "java",
		Version:      "1.1.1",
		VPodCapacity: 5,
	}, arkService, node)
	assert.Assert(t, len(node.Labels) == 2)
	assert.Assert(t, len(node.Spec.Taints) == 1)
	assert.Assert(t, node.Status.Phase == corev1.NodeRunning)
}

func TestModelUtils_CmpBizModel(t *testing.T) {
	bizModel1 := &ark.BizModel{
		BizName:    "test-biz1",
		BizVersion: "0.0.1",
	}
	bizModel2 := &ark.BizModel{
		BizName:    "test-biz1",
		BizVersion: "0.0.2",
	}
	bizModel3 := &ark.BizModel{
		BizName:    "test-biz2",
		BizVersion: "0.0.1",
	}
	bizModel4 := &ark.BizModel{
		BizName:    "test-biz2",
		BizVersion: "0.0.2",
	}
	bizList := []*ark.BizModel{
		bizModel1,
		bizModel2,
		bizModel3,
		bizModel4,
	}
	for i, bizModel := range bizList {
		assert.Assert(t, moduleUtils.CmpBizModel(bizModel, bizModel))
		for _, bizModelNext := range bizList[i+1:] {
			assert.Assert(t, !moduleUtils.CmpBizModel(bizModelNext, bizModel))
		}
	}
}

func TestModelUtils_GetBizIdentityFromBizInfo(t *testing.T) {
	assert.Assert(t, moduleUtils.GetBizIdentityFromBizInfo(&ark.ArkBizInfo{
		BizName:        "test-biz",
		BizState:       "ACTIVATE",
		BizVersion:     "1.1.1",
		MainClass:      "Test",
		WebContextPath: "Test",
	}) == "test-biz:1.1.1")
}

func TestModelUtils_GetBizIdentityFromBizModel(t *testing.T) {
	assert.Assert(t, moduleUtils.GetBizIdentityFromBizModel(&ark.BizModel{
		BizName:    "test-biz",
		BizVersion: "0.0.1",
		BizUrl:     "file:///test/test1.jar",
	}) == "test-biz:0.0.1")
}

func TestModelUtils_TranslateCoreV1ContainerToBizModel(t *testing.T) {
	bizModel := moduleUtils.TranslateCoreV1ContainerToBizModel(corev1.Container{
		Name:       "test_container",
		Image:      "file:///test/test1",
		WorkingDir: "/home",
		Env: []corev1.EnvVar{
			{
				Name:  "BIZ_VERSION",
				Value: "1.1.1",
			},
		},
	})
	assert.Assert(t, bizModel.BizUrl == "file:///test/test1")
	assert.Assert(t, bizModel.BizName == "test_container")
	assert.Assert(t, bizModel.BizVersion == "1.1.1")
}

func TestModelUtils_GetBizModelsFromCoreV1Pod(t *testing.T) {
	bizModelList := moduleUtils.GetBizModelsFromCoreV1Pod(&corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:       "test_container",
					Image:      "file:///test/test1",
					WorkingDir: "/home",
					Env: []corev1.EnvVar{
						{
							Name:  "BIZ_VERSION",
							Value: "1.1.1",
						},
					},
				},
				{
					Name:       "test_container-2",
					Image:      "file:///test/test2",
					WorkingDir: "/home",
					Env: []corev1.EnvVar{
						{
							Name:  "BIZ_VERSION",
							Value: "1.1.2",
						},
					},
				},
			},
		},
	})
	assert.Assert(t, len(bizModelList) == 2)
}

func TestModelUtils_GetPodKey(t *testing.T) {
	assert.Assert(t, moduleUtils.GetPodKey(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
		},
	}) == "test-namespace/test-pod")
}

func TestModelUtils_TranslateArkBizInfoToV1ContainerStatus(t *testing.T) {
	bizModel := &ark.BizModel{
		BizName:    "test-biz",
		BizVersion: "1.1.1",
		BizUrl:     "file:///test/test1.jar",
	}
	var infoNotInstalled *ark.ArkBizInfo
	infoActivated := &ark.ArkBizInfo{
		BizName:    "test-biz",
		BizState:   "ACTIVATED",
		BizVersion: "1.1.1",
	}
	infoResolved := &ark.ArkBizInfo{
		BizName:    "test-biz",
		BizState:   "RESOLVED",
		BizVersion: "1.1.1",
	}
	infoDeactivated := &ark.ArkBizInfo{
		BizName:    "test-biz",
		BizState:   "DEACTIVATED",
		BizVersion: "1.1.1",
	}
	assert.Assert(t, moduleUtils.TranslateArkBizInfoToV1ContainerStatus(bizModel, infoNotInstalled, true).State.Waiting.Reason == "BizPending")
	assert.Assert(t, moduleUtils.TranslateArkBizInfoToV1ContainerStatus(bizModel, infoResolved, true).State.Waiting.Reason == "BizResolved")
	assert.Assert(t, moduleUtils.TranslateArkBizInfoToV1ContainerStatus(bizModel, infoActivated, true).State.Running != nil)
	assert.Assert(t, moduleUtils.TranslateArkBizInfoToV1ContainerStatus(bizModel, infoDeactivated, true).State.Terminated != nil)
	assert.Assert(t, moduleUtils.TranslateArkBizInfoToV1ContainerStatus(bizModel, infoNotInstalled, false).State.Waiting.Reason == "BaseDown")
}

func TestModelUtils_TranslateContainerToSummaryContainerStats(t *testing.T) {
	// TODO implement later
	containerStats := moduleUtils.TranslateContainerToSummaryContainerStats(&corev1.Container{})
	assert.Assert(t, containerStats.Name == "")
}

func TestModelUtils_TranslatePodToSummaryPodStats(t *testing.T) {
	podStats := moduleUtils.TranslatePodToSummaryPodStats(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			UID:       "test-pod-uid",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "file:///test/test1.jar",
				},
			},
		},
		Status: corev1.PodStatus{
			StartTime: &metav1.Time{},
		},
	})
	assert.Assert(t, podStats.PodRef.Name == "test-pod")
	assert.Assert(t, podStats.PodRef.Namespace == "test-namespace")
	assert.Assert(t, podStats.PodRef.UID == "test-pod-uid")
	assert.Assert(t, len(podStats.Containers) == 1)
}
