package inspection

import (
	"context"
	"github.com/koupleless/virtual-kubelet/model"
	"gotest.tools/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
	"time"
)

func TestPodScheduleInspection_Register(t *testing.T) {
	inspection := PodScheduleInspection{}
	inspection.Register(nil)
}

func TestPodScheduleInspection_Inspect(t *testing.T) {
	inspection := PodScheduleInspection{}
	clientset := fake.NewSimpleClientset(&v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pod",
			Labels: map[string]string{
				model.LabelKeyOfModuleControllerComponent: model.ModuleControllerComponentModule,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					UID: "test-uid",
				},
			},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "test-container",
					Image: "test-image",
				},
			},
		},
		Status: v1.PodStatus{
			Phase: v1.PodPending,
			Conditions: []v1.PodCondition{
				{
					Type:    v1.PodScheduled,
					Status:  v1.ConditionFalse,
					Message: "test-message",
				},
			},
		},
	})
	inspection.Register(clientset)
	inspection.Inspect(context.Background())
	inspection.Inspect(context.Background())
}

func TestPodScheduleInspection_GetIntervalMilliSec(t *testing.T) {
	inspection := PodScheduleInspection{}
	sec := inspection.GetInterval()
	assert.Equal(t, time.Minute, sec)
}

func TestOwnerReported(t *testing.T) {
	inspection := PodScheduleInspection{}
	inspection.Register(nil)
	assert.Equal(t, inspection.ownerReported("test-uid", 10), false)
	assert.Equal(t, inspection.ownerReported("test-uid", 10), true)
}
