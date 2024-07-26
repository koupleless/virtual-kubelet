package utils

import (
	"github.com/koupleless/arkctl/v1/service/ark"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"testing"
)

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
		assert.Assert(t, ModelUtil.CmpBizModel(bizModel, bizModel))
		for _, bizModelNext := range bizList[i+1:] {
			assert.Assert(t, !ModelUtil.CmpBizModel(bizModelNext, bizModel))
		}
	}
}

func TestModelUtils_GetBizIdentityFromBizInfo(t *testing.T) {
	assert.Assert(t, ModelUtil.GetBizIdentityFromBizInfo(&ark.ArkBizInfo{
		BizName:        "test-biz",
		BizState:       "ACTIVATE",
		BizVersion:     "1.1.1",
		MainClass:      "Test",
		WebContextPath: "Test",
	}) == "test-biz:1.1.1")
}

func TestModelUtils_GetBizIdentityFromBizModel(t *testing.T) {
	assert.Assert(t, ModelUtil.GetBizIdentityFromBizModel(&ark.BizModel{
		BizName:    "test-biz",
		BizVersion: "0.0.1",
		BizUrl:     "file:///test/test1.jar",
	}) == "test-biz:0.0.1")
}

func TestModelUtils_TranslateCoreV1ContainerToBizModel(t *testing.T) {
	bizModel := ModelUtil.TranslateCoreV1ContainerToBizModel(corev1.Container{
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
	bizModelList := ModelUtil.GetBizModelsFromCoreV1Pod(&corev1.Pod{
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
	assert.Assert(t, ModelUtil.GetPodKey(&corev1.Pod{
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
		BizStateRecords: []ark.ArkBizStateRecord{
			{
				ChangeTime: "2024-07-09 16:48:56.921",
				State:      "ACTIVATED",
				Reason:     "test reason",
				Message:    "test message",
			},
			{
				ChangeTime: "2024-07-09 16:48:56.921",
				State:      "RESOLVED",
				Reason:     "test reason",
				Message:    "test message",
			},
		},
	}
	infoResolved := &ark.ArkBizInfo{
		BizName:    "test-biz",
		BizState:   "RESOLVED",
		BizVersion: "1.1.1",
		BizStateRecords: []ark.ArkBizStateRecord{
			{
				ChangeTime: "2024-07-09 16:48:56.921",
				State:      "RESOLVED",
				Reason:     "test reason",
				Message:    "test message",
			},
		},
	}
	infoDeactivated := &ark.ArkBizInfo{
		BizName:    "test-biz",
		BizState:   "DEACTIVATED",
		BizVersion: "1.1.1",
		BizStateRecords: []ark.ArkBizStateRecord{
			{
				ChangeTime: "2024-07-09 16:48:56.921",
				State:      "DEACTIVATED",
				Reason:     "test reason",
				Message:    "test message",
			},
			{
				ChangeTime: "2024-07-09 16:48:56.921",
				State:      "ACTIVATED",
				Reason:     "test reason",
				Message:    "test message",
			},
			{
				ChangeTime: "2024-07-09 16:48:56.921",
				State:      "RESOLVED",
				Reason:     "test reason",
				Message:    "test message",
			},
		},
	}
	assert.Assert(t, ModelUtil.TranslateArkBizInfoToV1ContainerStatus(bizModel, infoNotInstalled).State.Waiting.Reason == "BizPending")
	assert.Assert(t, ModelUtil.TranslateArkBizInfoToV1ContainerStatus(bizModel, infoResolved).State.Waiting.Reason == "BizResolved")
	assert.Assert(t, ModelUtil.TranslateArkBizInfoToV1ContainerStatus(bizModel, infoActivated).State.Running != nil)
	assert.Assert(t, ModelUtil.TranslateArkBizInfoToV1ContainerStatus(bizModel, infoDeactivated).State.Terminated != nil)
}

func TestModelUtils_TranslateArkBizInfoToV1ContainerStatus_ACTIVATEDButChangeTimeNotProvided(t *testing.T) {
	bizModel := &ark.BizModel{
		BizName:    "test-biz",
		BizVersion: "1.1.1",
		BizUrl:     "file:///test/test1.jar",
	}
	infoActivated := &ark.ArkBizInfo{
		BizName:    "test-biz",
		BizState:   "ACTIVATED",
		BizVersion: "1.1.1",
		BizStateRecords: []ark.ArkBizStateRecord{
			{
				ChangeTime: "",
				State:      "ACTIVATED",
				Reason:     "test reason",
				Message:    "test message",
			},
		},
	}
	assert.Assert(t, ModelUtil.TranslateArkBizInfoToV1ContainerStatus(bizModel, infoActivated).State.Running.StartedAt.UnixMilli() == 0)
}

func TestModelUtils_TranslateArkBizInfoToV1ContainerStatus_ACTIVATEDButChangeTimeNotValid(t *testing.T) {
	bizModel := &ark.BizModel{
		BizName:    "test-biz",
		BizVersion: "1.1.1",
		BizUrl:     "file:///test/test1.jar",
	}
	infoActivated := &ark.ArkBizInfo{
		BizName:    "test-biz",
		BizState:   "ACTIVATED",
		BizVersion: "1.1.1",
		BizStateRecords: []ark.ArkBizStateRecord{
			{
				ChangeTime: "2024-07-09 16:48:56",
				State:      "ACTIVATED",
				Reason:     "test reason",
				Message:    "test message",
			},
		},
	}
	assert.Assert(t, ModelUtil.TranslateArkBizInfoToV1ContainerStatus(bizModel, infoActivated).State.Running.StartedAt.UnixMilli() == 0)
}

func TestPodsEqual_ContainersNotEqual(t *testing.T) {
	assert.Assert(t, !PodsEqual(&corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "test1",
				},
			},
		},
	}, &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "test2",
				},
			},
		},
	}))
}

func TestPodsEqual_InitContainersNotEqual(t *testing.T) {
	assert.Assert(t, !PodsEqual(&corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name: "test1",
				},
			},
		},
	}, &corev1.Pod{
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name: "test2",
				},
			},
		},
	}))
}

func TestPodsEqual_ActiveDeadlineSecondsNotEqual(t *testing.T) {
	assert.Assert(t, !PodsEqual(&corev1.Pod{
		Spec: corev1.PodSpec{
			ActiveDeadlineSeconds: ptr.To[int64](600),
		},
	}, &corev1.Pod{
		Spec: corev1.PodSpec{
			ActiveDeadlineSeconds: ptr.To[int64](601),
		},
	}))
}

func TestPodsEqual_TolerationsNotEqual(t *testing.T) {
	assert.Assert(t, !PodsEqual(&corev1.Pod{
		Spec: corev1.PodSpec{
			Tolerations: []corev1.Toleration{
				{
					Key:               "test",
					Operator:          "",
					Value:             "",
					Effect:            "",
					TolerationSeconds: nil,
				},
			},
		},
	}, &corev1.Pod{
		Spec: corev1.PodSpec{
			Tolerations: []corev1.Toleration{
				{
					Key:               "test1",
					Operator:          "",
					Value:             "",
					Effect:            "",
					TolerationSeconds: nil,
				},
			},
		},
	}))
}

func TestPodsEqual_LabelsNotEqual(t *testing.T) {
	assert.Assert(t, !PodsEqual(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"testLabel": "testValue",
			},
		},
	}, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"testLabel": "testValue1",
			},
		},
	}))
}

func TestPodsEqual_AnnotationsNotEqual(t *testing.T) {
	assert.Assert(t, !PodsEqual(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"testAnnotation": "testValue",
			},
		},
	}, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"testAnnotation": "testValue1",
			},
		},
	}))
}

func TestPodsEqual_Equal(t *testing.T) {
	assert.Assert(t, PodsEqual(&corev1.Pod{}, &corev1.Pod{}))
}
