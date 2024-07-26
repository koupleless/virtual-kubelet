package pod_provider

import (
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

var runtimeInfoStore *RuntimeInfoStore

var defaultPod = &corev1.Pod{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "test-Namespace",
		Name:      "test-defaultPod",
	},
	Spec: corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "test-container1",
				Image: "file:///test/test1.jar",
				Env: []corev1.EnvVar{
					{
						Name:  "BIZ_VERSION",
						Value: "1.1.1",
					},
				},
			}, {
				Name:  "test-container2",
				Image: "file:///test/test2.jar",
				Env: []corev1.EnvVar{
					{
						Name:  "BIZ_VERSION",
						Value: "1.1.2",
					},
				},
			},
		},
	},
}

var defaultPod2 = &corev1.Pod{
	ObjectMeta: metav1.ObjectMeta{
		Namespace: "test-Namespace-2",
		Name:      "test-defaultPod-2",
	},
	Spec: corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "test-container3",
				Image: "file:///test/test3.jar",
				Env: []corev1.EnvVar{
					{
						Name:  "BIZ_VERSION",
						Value: "1.1.3",
					},
				},
			},
		},
	},
}

func TestNewRuntimeInfoStore(t *testing.T) {
	runtimeInfoStore = NewRuntimeInfoStore()
	assert.Assert(t, runtimeInfoStore != nil)
}

func TestRuntimeInfoStore_PutPod(t *testing.T) {
	podKey := utils.ModelUtil.GetPodKey(defaultPod)
	runtimeInfoStore.PutPod(defaultPod)
	assert.Assert(t, len(runtimeInfoStore.GetPods()) == 1)
	assert.Assert(t, runtimeInfoStore.GetPods()[0].Name == "test-defaultPod")
	assert.Assert(t, len(runtimeInfoStore.GetRelatedBizModels(podKey)) == 2)
	assert.Assert(t, runtimeInfoStore.GetRelatedPodKeyByBizIdentity(runtimeInfoStore.getBizIdentity(&ark.BizModel{
		BizName:    "test-container1",
		BizVersion: "1.1.1",
	})) == podKey)
}

func TestRuntimeInfoStore_GetBizModel(t *testing.T) {
	model := runtimeInfoStore.GetBizModel(runtimeInfoStore.getBizIdentity(&ark.BizModel{
		BizName:    "test-container1",
		BizVersion: "1.1.2",
	}))
	assert.Assert(t, model == nil)
	model = runtimeInfoStore.GetBizModel(runtimeInfoStore.getBizIdentity(&ark.BizModel{
		BizName:    "test-container1",
		BizVersion: "1.1.1",
	}))
	assert.Assert(t, model != nil)
}

func TestRuntimeInfoStore_GetPodByKey(t *testing.T) {
	podKey := utils.ModelUtil.GetPodKey(defaultPod)
	podKey2 := utils.ModelUtil.GetPodKey(defaultPod2)
	pod := runtimeInfoStore.GetPodByKey(podKey)
	pod2 := runtimeInfoStore.GetPodByKey(podKey2)
	assert.Assert(t, pod != nil)
	assert.Assert(t, pod2 == nil)
}

func TestRuntimeInfoStore_GetPods(t *testing.T) {
	pods := runtimeInfoStore.GetPods()
	assert.Assert(t, len(pods) == 1)
	assert.Assert(t, pods[0].Name == "test-defaultPod")
}

func TestRuntimeInfoStore_GetRelatedBizModels(t *testing.T) {
	podKey1 := utils.ModelUtil.GetPodKey(defaultPod)
	podKey2 := utils.ModelUtil.GetPodKey(defaultPod2)
	bizModels1 := runtimeInfoStore.GetRelatedBizModels(podKey1)
	bizModels2 := runtimeInfoStore.GetRelatedBizModels(podKey2)
	assert.Assert(t, len(bizModels1) == 2)
	assert.Assert(t, len(bizModels2) == 0)
}

func TestRuntimeInfoStore_GetRelatedPodKeyByBizIdentity(t *testing.T) {
	podKey := runtimeInfoStore.GetRelatedPodKeyByBizIdentity(utils.ModelUtil.GetBizIdentityFromBizModel(&ark.BizModel{
		BizName:    "test-container3",
		BizVersion: "1.1.3",
	}))
	assert.Assert(t, podKey == "")
}

func TestRuntimeInfoStore_DeletePod(t *testing.T) {
	runtimeInfoStore = NewRuntimeInfoStore()
	runtimeInfoStore.PutPod(defaultPod)
	runtimeInfoStore.DeletePod(runtimeInfoStore.getPodKey(defaultPod))
	assert.Assert(t, len(runtimeInfoStore.GetPods()) == 0)
}
