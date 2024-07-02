package let

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/java/model"
	"github.com/virtual-kubelet/virtual-kubelet/node/api"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

var baseProvider *BaseProvider

func TestNewBaseProvider(t *testing.T) {
	service := ark.BuildService(context.Background())
	baseProvider = NewBaseProvider("default", model.DefaultArkServicePort, service, nil)
	assert.Assert(t, baseProvider != nil)
	assert.Equal(t, baseProvider.namespace, "default")
}

func TestBaseProvider_Run(t *testing.T) {
	ctx := context.WithValue(context.Background(), "env", "test")
	baseProvider.Run(ctx)
}

func TestBaseProvider_CreatePod(t *testing.T) {
	ctx := context.WithValue(context.Background(), "env", "test")
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			UID:       "test-pod-uid",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "biz1",
					Image: "https://serverless-opensource.oss-cn-shanghai.aliyuncs.com/module-packages/stable/biz1-web-single-host-0.0.1-SNAPSHOT-ark-biz.jar",
					Env: []corev1.EnvVar{
						{
							Name:  "BIZ_VERSION",
							Value: "0.0.1-SNAPSHOT",
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			StartTime: &metav1.Time{Time: time.Now()},
		},
	}

	// uninstall test env bizs installed
	bizInfos, err := baseProvider.queryAllBiz(ctx)
	assert.NilError(t, err)
	for _, bizInfo := range bizInfos {
		if bizInfo.BizState != "DEACTIVATED" {
			err = baseProvider.unInstallBiz(ctx, &ark.BizModel{
				BizName:    bizInfo.BizName,
				BizVersion: bizInfo.BizVersion,
			})
			assert.NilError(t, err)
		}
	}

	err = baseProvider.CreatePod(ctx, pod)
	assert.NilError(t, err)

	// watch biz info states
	for i := 0; i < 60; i++ {
		time.Sleep(time.Second)
		bizInfo, err := baseProvider.queryBiz(context.Background(), baseProvider.runtimeInfoStore.modelUtils.GetBizIdentityFromBizModel(&ark.BizModel{
			BizName:    "biz1",
			BizVersion: "0.0.1-SNAPSHOT",
		}))
		assert.NilError(t, err)
		assert.Assert(t, bizInfo.BizState != "DEACTIVATED")
		if bizInfo.BizState == "ACTIVATED" {
			break
		}
	}
}

func TestBaseProvider_UpdatePod(t *testing.T) {
	ctx := context.WithValue(context.Background(), "env", "test")

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			UID:       "test-pod-uid",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "biz2",
					Image: "https://serverless-opensource.oss-cn-shanghai.aliyuncs.com/module-packages/stable/biz2-web-single-host-0.0.1-SNAPSHOT-ark-biz.jar",
					Env: []corev1.EnvVar{
						{
							Name:  "BIZ_VERSION",
							Value: "0.0.1-SNAPSHOT",
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			StartTime: &metav1.Time{Time: time.Now()},
		},
	}

	err := baseProvider.UpdatePod(ctx, pod)
	assert.NilError(t, err)

	// watch biz info states
	for i := 0; i < 60; i++ {
		time.Sleep(time.Second)
		bizInfo2, err := baseProvider.queryBiz(context.Background(), baseProvider.modelUtils.GetBizIdentityFromBizModel(&ark.BizModel{
			BizName:    "biz2",
			BizVersion: "0.0.1-SNAPSHOT",
		}))
		assert.NilError(t, err)
		assert.Assert(t, bizInfo2.BizState != "DEACTIVATED")
		if bizInfo2.BizState == "ACTIVATED" {
			break
		} else {
			podStatus, err := baseProvider.GetPodStatus(ctx, "test-namespace", "test-pod")
			assert.NilError(t, err)
			assert.Assert(t, podStatus != nil)
		}
	}
}

func TestBaseProvider_AttachToContainer(t *testing.T) {
	ctx := context.WithValue(context.Background(), "env", "test")
	defer func() {
		if r := recover(); r != nil {
			assert.Assert(t, true)
		}
	}()
	err := baseProvider.AttachToContainer(ctx, "", "", "", nil)
	assert.NilError(t, err)
	assert.Assert(t, false)
}

func TestBaseProvider_GetContainerLogs(t *testing.T) {
	ctx := context.WithValue(context.Background(), "env", "test")
	// TODO implement later
	logs, err := baseProvider.GetContainerLogs(ctx, "", "", "", api.ContainerLogOpts{})
	assert.NilError(t, err)
	assert.Assert(t, logs == nil)
}

func TestBaseProvider_GetMetricsResource(t *testing.T) {
	ctx := context.WithValue(context.Background(), "env", "test")
	// TODO implement later
	metrics, err := baseProvider.GetMetricsResource(ctx)
	assert.NilError(t, err)
	assert.Assert(t, len(metrics) == 0)
}

func TestBaseProvider_GetPod(t *testing.T) {
	ctx := context.WithValue(context.Background(), "env", "test")
	pod, err := baseProvider.GetPod(ctx, "test-namespace", "test-pod")
	assert.NilError(t, err)
	assert.Assert(t, pod != nil)
	assert.Assert(t, len(pod.Spec.Containers) != 0)
	assert.Assert(t, pod.Spec.Containers[0].Name == "biz2")
}

func TestBaseProvider_GetPods(t *testing.T) {
	ctx := context.WithValue(context.Background(), "env", "test")
	pods, err := baseProvider.GetPods(ctx)
	assert.NilError(t, err)
	assert.Assert(t, len(pods) != 0)
}

func TestBaseProvider_GetPodStatus(t *testing.T) {
	ctx := context.WithValue(context.Background(), "env", "test")
	podStatus, err := baseProvider.GetPodStatus(ctx, "test-namespace", "test-pod")
	assert.NilError(t, err)
	assert.Assert(t, podStatus != nil)
	assert.Assert(t, len(podStatus.ContainerStatuses) != 0)
}

func TestBaseProvider_GetStatsSummary(t *testing.T) {
	ctx := context.WithValue(context.Background(), "env", "test")
	summary, err := baseProvider.GetStatsSummary(ctx)
	assert.NilError(t, err)
	assert.Assert(t, summary != nil)
}

func TestBaseProvider_PortForward(t *testing.T) {
	ctx := context.WithValue(context.Background(), "env", "test")
	defer func() {
		if r := recover(); r != nil {
			assert.Assert(t, true)
		}
	}()
	err := baseProvider.PortForward(ctx, "", "", 1234, nil)
	assert.NilError(t, err)
	assert.Assert(t, false)
}

func TestBaseProvider_RunInContainer(t *testing.T) {
	ctx := context.WithValue(context.Background(), "env", "test")
	defer func() {
		if r := recover(); r != nil {
			assert.Assert(t, true)
		}
	}()
	err := baseProvider.RunInContainer(ctx, "", "", "", []string{}, nil)
	assert.NilError(t, err)
	assert.Assert(t, false)
}

func TestBaseProvider_DeletePod(t *testing.T) {
	ctx := context.WithValue(context.Background(), "env", "test")
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			UID:       "test-pod-uid",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "biz2",
					Image: "https://serverless-opensource.oss-cn-shanghai.aliyuncs.com/module-packages/stable/biz2-web-single-host-0.0.1-SNAPSHOT-ark-biz.jar",
					Env: []corev1.EnvVar{
						{
							Name:  "BIZ_VERSION",
							Value: "0.0.1-SNAPSHOT",
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{},
	}
	err := baseProvider.DeletePod(ctx, pod)
	assert.NilError(t, err)
}
