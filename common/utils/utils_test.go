package utils

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/koupleless/virtual-kubelet/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

func TestTimedTaskWithInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	counter := 0
	go TimedTaskWithInterval(ctx, time.Second, func(ctx context.Context) {
		counter++
	})
	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, 1, counter)
	time.Sleep(time.Second)
	assert.Equal(t, 2, counter)
}

func TestGetPodKey(t *testing.T) {
	key := GetPodKey(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "suite",
			Namespace: "suite",
		},
	})
	assert.Equal(t, "suite/suite", key)
}

func TestGetEnv(t *testing.T) {
	env := GetEnv("TEST_ENV", "default")
	assert.Equal(t, "default", env)
	os.Setenv("TEST_ENV", "suite")
	defer os.Unsetenv("TEST_ENV")
	env = GetEnv("TEST_ENV", "default")
	assert.Equal(t, "suite", env)
}

func TestCheckAndFinallyCall_NoTimeout(t *testing.T) {
	status := false
	checkTimes := 0
	finally := false
	timeout := false
	go CheckAndFinallyCall(context.Background(), func(context.Context) (bool, error) {
		checkTimes++
		return status, nil
	}, time.Second*10, time.Second, func() {
		finally = true
	}, func() {
		timeout = true
	})
	time.Sleep(time.Second + time.Millisecond*100)
	assert.Equal(t, 1, checkTimes)
	status = true
	time.Sleep(time.Second)
	assert.Equal(t, 2, checkTimes)
	assert.Equal(t, true, finally)
	assert.Equal(t, false, timeout)
}

func TestCheckAndFinallyCall_Timeout(t *testing.T) {
	status := false
	checkTimes := 0
	finally := false
	timeout := false
	go CheckAndFinallyCall(context.Background(), func(context.Context) (bool, error) {
		checkTimes++
		return status, nil
	}, time.Second*2+time.Millisecond*100, time.Second, func() {
		finally = true
	}, func() {
		timeout = true
	})
	time.Sleep(time.Second * 3)
	assert.Equal(t, 2, checkTimes)
	assert.Equal(t, false, finally)
	assert.Equal(t, true, timeout)
}

func TestPodsEqual(t *testing.T) {
	assert.Equal(t, false, PodsEqual(&corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "suite", Image: "suite"}}},
	}, &corev1.Pod{
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "test1", Image: "suite"}}},
	}))
	assert.Equal(t, false, PodsEqual(&corev1.Pod{
		Spec: corev1.PodSpec{InitContainers: []corev1.Container{{Name: "suite", Image: "suite"}}},
	}, &corev1.Pod{
		Spec: corev1.PodSpec{InitContainers: []corev1.Container{{Name: "test1", Image: "suite"}}},
	}))
	assert.Equal(t, false, PodsEqual(&corev1.Pod{
		Spec: corev1.PodSpec{ActiveDeadlineSeconds: ptr.To[int64](1)},
	}, &corev1.Pod{
		Spec: corev1.PodSpec{ActiveDeadlineSeconds: ptr.To[int64](2)},
	}))
	assert.Equal(t, false, PodsEqual(&corev1.Pod{
		Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{Key: "suite"}}},
	}, &corev1.Pod{
		Spec: corev1.PodSpec{Tolerations: []corev1.Toleration{{Key: "test2"}}},
	}))
	assert.Equal(t, false, PodsEqual(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"suite": "suite"},
		},
	}, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"suite": "test1"},
		},
	}))
	assert.Equal(t, false, PodsEqual(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"suite": "suite"},
		},
	}, &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"suite": "test1"},
		},
	}))
	assert.Equal(t, true, PodsEqual(&corev1.Pod{}, &corev1.Pod{}))
}

func TestConvertByteNumToResourceQuantity_LTZero(t *testing.T) {
	quantity := ConvertByteNumToResourceQuantity(-1)
	assert.True(t, quantity.IsZero())
}

func TestConvertByteNumToResourceQuantity_GTZero(t *testing.T) {
	quantity := ConvertByteNumToResourceQuantity(1024)
	assert.Equal(t, int64(1024), quantity.Value())
}

func TestGetPodKeyFromContainerKey(t *testing.T) {
	assert.Equal(t, "/suite", GetPodKeyFromContainerKey("/suite/suite"))
}

func TestGetContainerNameFromContainerKey(t *testing.T) {
	assert.Equal(t, "suite", GetContainerNameFromContainerKey("/suite/suite"))
}

func TestGetContainerKey(t *testing.T) {
	assert.Equal(t, "suite/suite/suite", GetContainerKey("suite/suite", "suite"))
}

func TestFormatNodeName(t *testing.T) {
	assert.Equal(t, model.VNodePrefix+".suite.suite", FormatNodeName("suite", "suite"))
}

func TestExtractNodeIDFromNodeName(t *testing.T) {
	assert.Equal(t, "suite", ExtractNodeIDFromNodeName("vnode.suite.suite"))
	assert.Equal(t, "", ExtractNodeIDFromNodeName("suite"))
}

func TestConvertBizStatusToContainerStatus_NoData(t *testing.T) {
	status, _ := ConvertBizStatusToContainerStatus(&corev1.Container{
		Name:  "suite",
		Image: "test_img",
	}, &corev1.ContainerStatus{Name: "suite"}, nil)
	assert.Equal(t, status.State, corev1.ContainerState{})
}

func TestConvertBizStatusToContainerStatus_UnResolved(t *testing.T) {
	status, _ := ConvertBizStatusToContainerStatus(&corev1.Container{
		Name:  "suite",
		Image: "test_img",
	}, &corev1.ContainerStatus{Name: "suite"}, &model.BizStatusData{
		Key:        "test_key",
		Name:       "suite",
		PodKey:     "pod_key",
		State:      string(model.BizStateUnResolved),
		ChangeTime: time.Now(),
		Reason:     "resolved",
		Message:    "resolved message",
	})
	assert.Equal(t, status.State.Waiting.Reason, model.StateReasonAwaitingResync)
}

func TestConvertBizStatusToContainerStatus_RESOLVED(t *testing.T) {
	status, _ := ConvertBizStatusToContainerStatus(&corev1.Container{
		Name:  "suite",
		Image: "test_img",
	}, nil, &model.BizStatusData{
		Key:        "test_key",
		Name:       "suite",
		PodKey:     "pod_key",
		State:      string(model.BizStateResolved),
		ChangeTime: time.Now(),
		Reason:     "resolved",
		Message:    "resolved message",
	})
	assert.NotNil(t, status.State.Waiting)
	assert.Equal(t, "resolved", status.State.Waiting.Reason)
	assert.Equal(t, "resolved message", status.State.Waiting.Message)
}

func TestConvertBizStatusToContainerStatus_ACTIVATED(t *testing.T) {
	status, _ := ConvertBizStatusToContainerStatus(&corev1.Container{
		Name:  "suite",
		Image: "test_img",
	}, nil, &model.BizStatusData{
		Key:        "test_key",
		Name:       "suite",
		PodKey:     "pod_key",
		State:      string(model.BizStateActivated),
		ChangeTime: time.Now(),
	})
	assert.NotNil(t, status.State.Running)
}

func TestConvertBizStatusToContainerStatus_DEACTIVED(t *testing.T) {
	status, _ := ConvertBizStatusToContainerStatus(&corev1.Container{
		Name:  "suite",
		Image: "test_img",
	}, nil, &model.BizStatusData{
		Key:        "test_key",
		Name:       "suite",
		PodKey:     "pod_key",
		State:      string(model.BizStateDeactivated),
		ChangeTime: time.Now(),
		Reason:     "deactivated",
		Message:    "deactivated message",
	})
	assert.NotNil(t, status.State.Waiting)
	assert.Equal(t, "deactivated", status.State.Waiting.Reason)
	assert.Equal(t, "deactivated message", status.State.Waiting.Message)
}

func TestSplitMetaNamespaceKey(t *testing.T) {
	namespace := ""
	name := ""
	var err error
	namespace, name, err = SplitMetaNamespaceKey("suite")
	assert.Equal(t, "", namespace)
	assert.Equal(t, "suite", name)
	assert.NoError(t, err)
	namespace, name, err = SplitMetaNamespaceKey("suite/test1")
	assert.Equal(t, "suite", namespace)
	assert.Equal(t, "test1", name)
	assert.NoError(t, err)
	namespace, name, err = SplitMetaNamespaceKey("suite/suite/suite")
	assert.Equal(t, "", namespace)
	assert.Equal(t, "", name)
	assert.Error(t, err)
}

func TestCallWithRetry_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := CallWithRetry(ctx, func(_ int) (shouldRetry bool, err error) {
		return true, nil
	}, nil)
	assert.Error(t, err)
}

func TestCallWithRetry_RetryAndSuccess(t *testing.T) {
	flag := 0
	err := CallWithRetry(context.Background(), func(_ int) (shouldRetry bool, err error) {
		flag++
		return flag < 2, nil
	}, nil)
	assert.NoError(t, err)
}

func TestCallWithRetry_RetryAndError(t *testing.T) {
	err := CallWithRetry(context.Background(), func(_ int) (shouldRetry bool, err error) {
		return false, errors.New("test error")
	}, nil)
	assert.Error(t, err)
}

func TestDefaultRateLimiter(t *testing.T) {
	assert.Equal(t, DefaultRateLimiter(1), 100*time.Millisecond)
	assert.Equal(t, DefaultRateLimiter(30), 90*time.Second)
	assert.Equal(t, DefaultRateLimiter(100), 1000*time.Second)
}

func TestFillPodKey(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ut-pod1",
				Namespace: "ut-ns",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "ut-biz1",
						Image: "ut-biz1-version1.jar",
						Env: []corev1.EnvVar{
							{
								Name:  "BIZ_VERSION",
								Value: "ut-biz1-version1",
							},
						},
					},
					{
						Name:  "ut-biz2",
						Image: "ut-biz2-version1.jar",
						Env: []corev1.EnvVar{
							{
								Name:  "BIZ_VERSION",
								Value: "ut-biz2-version1",
							},
						},
					},
				},
			},
		}, {
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ut-pod2",
				Namespace: "ut-ns",
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "ut-biz1",
						Image: "ut-biz1-version2.jar",
						Env: []corev1.EnvVar{
							{
								Name:  "BIZ_VERSION",
								Value: "ut-biz1-version2",
							},
						},
					},
					{
						Name:  "ut-biz2",
						Image: "ut-biz2-version2.jar",
						Env: []corev1.EnvVar{
							{
								Name:  "BIZ_VERSION",
								Value: "ut-biz2-version2",
							},
						},
					},
				},
			},
		},
	}

	bizStatusDatas := []model.BizStatusData{
		{
			Key:  "ut-biz1:ut-biz1-version1",
			Name: "ut-biz1",
		}, {
			Key:  "ut-biz2:ut-biz2-version2",
			Name: "ut-biz2",
		},
	}

	bizStatusDatasWithPodKey, bizStatusDatasWithNoPodKey := FillPodKey(pods, bizStatusDatas)
	assert.Equal(t, "ut-ns/ut-pod1", bizStatusDatasWithPodKey[0].PodKey)
	assert.Equal(t, "ut-ns/ut-pod2", bizStatusDatasWithPodKey[1].PodKey)
	assert.Equal(t, len(bizStatusDatasWithNoPodKey), 0)
}

func TestOrElse(t *testing.T) {
	assert.Equal(t, "default", OrElse("", "default"))
	assert.EqualValues(t, 1, OrElse(0, 1))
	assert.EqualValues(t, 1.0, OrElse(0, 1.0))
	assert.EqualValues(t, ptr.To(1), OrElse(nil, ptr.To(1)))
	assert.Equal(t, "value", OrElse("value", "default"))
}
