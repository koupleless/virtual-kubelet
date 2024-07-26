// Copyright © 2017 The virtual-kubelet authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package virtual_kubelet

import (
	"context"
	"fmt"
	kubeinformers "k8s.io/client-go/informers"
	v12 "k8s.io/client-go/informers/core/v1"
	"testing"
	"time"

	testutil "github.com/koupleless/virtual-kubelet/common/testutil/util"
	"golang.org/x/time/rate"
	"gotest.tools/assert"
	is "gotest.tools/assert/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/util/workqueue"
)

type TestController struct {
	*PodController
	mock         *mockProviderAsync
	client       *fake.Clientset
	podsInformer v12.PodInformer
}

func newTestController() *TestController {
	fk8s := fake.NewSimpleClientset()

	iFactory := kubeinformers.NewSharedInformerFactoryWithOptions(fk8s, 10*time.Minute)
	podsInformer := iFactory.Core().V1().Pods()
	p := newMockProvider()
	podController, err := NewPodController(PodControllerConfig{
		PodClient:     fk8s.CoreV1(),
		PodLister:     podsInformer.Lister(),
		PodInformer:   podsInformer,
		EventRecorder: testutil.FakeEventRecorder(5),
		Provider:      p,
		SyncPodsFromKubernetesRateLimiter: workqueue.NewMaxOfRateLimiter(
			// The default upper bound is 1000 seconds. Let's not use that.
			workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 10*time.Millisecond),
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		),
		SyncPodStatusFromProviderRateLimiter: workqueue.NewMaxOfRateLimiter(
			// The default upper bound is 1000 seconds. Let's not use that.
			workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 10*time.Millisecond),
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		),
		DeletePodsFromKubernetesRateLimiter: workqueue.NewMaxOfRateLimiter(
			// The default upper bound is 1000 seconds. Let's not use that.
			workqueue.NewItemExponentialFailureRateLimiter(5*time.Millisecond, 10*time.Millisecond),
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		),
	})

	if err != nil {
		panic(err)
	}

	return &TestController{
		PodController: podController,
		mock:          p,
		client:        fk8s,
		podsInformer:  podsInformer,
	}
}

// Run starts the informer and runs the pod controller
func (tc *TestController) Run(ctx context.Context, n int) error {
	return tc.PodController.Run(ctx, n)
}

func TestPodCreateNewPod(t *testing.T) {
	svr := newTestController()

	pod := &corev1.Pod{}
	pod.ObjectMeta.Namespace = "default" //nolint:goconst
	pod.ObjectMeta.Name = "nginx"        //nolint:goconst
	pod.Spec = newPodSpec()

	err := svr.createOrUpdatePod(context.Background(), pod.DeepCopy())

	assert.Check(t, is.Nil(err))
	// createOrUpdate called CreatePod but did not call UpdatePod because the pod did not exist
	assert.Check(t, is.Equal(svr.mock.creates.read(), 1))
	assert.Check(t, is.Equal(svr.mock.updates.read(), 0))
}

func TestPodUpdateExisting(t *testing.T) {
	svr := newTestController()

	pod := &corev1.Pod{}
	pod.ObjectMeta.Namespace = "default"
	pod.ObjectMeta.Name = "nginx"
	pod.Spec = newPodSpec()

	err := svr.createOrUpdatePod(context.Background(), pod.DeepCopy())
	assert.Check(t, is.Nil(err))
	assert.Check(t, is.Equal(svr.mock.creates.read(), 1))
	assert.Check(t, is.Equal(svr.mock.updates.read(), 0))

	pod2 := pod.DeepCopy()
	pod2.Spec.Containers[0].Image = "nginx:1.15.12-perl"

	err = svr.createOrUpdatePod(context.Background(), pod2.DeepCopy())
	assert.Check(t, is.Nil(err))

	// createOrUpdate didn't call CreatePod but did call UpdatePod because the spec changed
	assert.Check(t, is.Equal(svr.mock.creates.read(), 1))
	assert.Check(t, is.Equal(svr.mock.updates.read(), 1))
}

func TestPodNoSpecChange(t *testing.T) {
	svr := newTestController()

	pod := &corev1.Pod{}
	pod.ObjectMeta.Namespace = "default"
	pod.ObjectMeta.Name = "nginx"
	pod.Spec = newPodSpec()

	err := svr.createOrUpdatePod(context.Background(), pod.DeepCopy())
	assert.Check(t, is.Nil(err))
	assert.Check(t, is.Equal(svr.mock.creates.read(), 1))
	assert.Check(t, is.Equal(svr.mock.updates.read(), 0))

	err = svr.createOrUpdatePod(context.Background(), pod.DeepCopy())
	assert.Check(t, is.Nil(err))

	// createOrUpdate didn't call CreatePod or UpdatePod, spec didn't change
	assert.Check(t, is.Equal(svr.mock.creates.read(), 1))
	assert.Check(t, is.Equal(svr.mock.updates.read(), 0))
}

func TestPodStatusDelete(t *testing.T) {
	ctx := context.Background()
	c := newTestController()
	pod := &corev1.Pod{}
	pod.ObjectMeta.Namespace = "default"
	pod.ObjectMeta.Name = "nginx"
	pod.Spec = newPodSpec()
	fk8s := fake.NewSimpleClientset(pod)
	c.client = fk8s
	c.PodController.client = fk8s.CoreV1()
	podCopy := pod.DeepCopy()
	deleteTime := v1.Time{Time: time.Now().Add(30 * time.Second)}
	podCopy.DeletionTimestamp = &deleteTime
	key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
	c.knownPods.Store(key, &knownPod{lastPodStatusReceivedFromProvider: podCopy})

	// test pod in provider delete
	err := c.updatePodStatus(ctx, pod, key)
	if err != nil {
		t.Fatal("pod updated failed")
	}
	newPod, err := c.client.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, v1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		t.Fatalf("Get pod %v failed", key)
	}
	if newPod != nil && newPod.DeletionTimestamp == nil {
		t.Fatalf("Pod %v delete failed", key)
	}
	t.Logf("pod delete success")

	// test pod in provider delete
	pod.DeletionTimestamp = &deleteTime
	if _, err = c.client.CoreV1().Pods(pod.Namespace).Create(ctx, pod, v1.CreateOptions{}); err != nil {
		t.Fatalf("Parepare pod in k8s failed, %v", err)
	}
	podCopy.Status.ContainerStatuses = []corev1.ContainerStatus{
		{
			State: corev1.ContainerState{
				Waiting: nil,
				Running: nil,
				Terminated: &corev1.ContainerStateTerminated{
					ExitCode: 1,
					Message:  "Exit",
				},
			},
		},
	}
	c.knownPods.Store(key, &knownPod{lastPodStatusReceivedFromProvider: podCopy})
	err = c.updatePodStatus(ctx, pod, key)
	if err != nil {
		t.Fatalf("pod updated failed %v", err)
	}
	newPod, err = c.client.CoreV1().Pods(pod.Namespace).Get(ctx, pod.Name, v1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		t.Fatalf("Get pod %v failed", key)
	}
	if newPod.DeletionTimestamp == nil {
		t.Fatalf("Pod %v delete failed", key)
	}
	if newPod.Status.ContainerStatuses[0].State.Terminated == nil {
		t.Fatalf("Pod status %v update failed", key)
	}
	t.Logf("pod updated, container status: %+v, pod delete Time: %v", newPod.Status.ContainerStatuses[0].State.Terminated, newPod.DeletionTimestamp)
}

func TestReCreatePodRace(t *testing.T) {
	ctx := context.Background()
	c := newTestController()
	pod := &corev1.Pod{}
	pod.ObjectMeta.Namespace = "default"
	pod.ObjectMeta.Name = "nginx"
	pod.Spec = newPodSpec()
	pod.UID = "aaaaa"
	podCopy := pod.DeepCopy()
	podCopy.UID = "bbbbb"

	// test conflict
	fk8s := &fake.Clientset{}
	c.client = fk8s
	c.PodController.client = fk8s.CoreV1()
	key := fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, pod.UID)
	c.knownPods.Store(key, &knownPod{lastPodStatusReceivedFromProvider: podCopy})
	c.deletePodsFromKubernetes.Enqueue(ctx, key)
	if err := c.podsInformer.Informer().GetStore().Add(pod); err != nil {
		t.Fatal(err)
	}
	c.client.AddReactor("delete", "pods", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		name := action.(core.DeleteAction).GetName()
		t.Logf("deleted pod %s", name)
		return true, nil, errors.NewConflict(schema.GroupResource{Group: "", Resource: "pods"}, "nginx", fmt.Errorf("test conflict"))
	})
	c.client.AddReactor("get", "pods", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		name := action.(core.GetAction).GetName()
		t.Logf("get pod %s", name)
		return true, podCopy, nil
	})

	err := c.deletePodsFromKubernetesHandler(ctx, key)
	if err != nil {
		t.Error("Failed")
	}
	p, err := c.client.CoreV1().Pods(podCopy.Namespace).Get(ctx, podCopy.Name, v1.GetOptions{})
	if err != nil {
		t.Fatalf("Pod not exist, %v", err)
	}
	if p.UID != podCopy.UID {
		t.Errorf("Desired uid: %v, get: %v", podCopy.UID, p.UID)
	}
	t.Log("pod conflict test success")

	// test not found
	c = newTestController()
	fk8s = &fake.Clientset{}
	c.client = fk8s
	c.knownPods.Store(key, &knownPod{lastPodStatusReceivedFromProvider: podCopy})
	c.deletePodsFromKubernetes.Enqueue(ctx, key)
	if err = c.podsInformer.Informer().GetStore().Add(pod); err != nil {
		t.Fatal(err)
	}
	c.client.AddReactor("delete", "pods", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		name := action.(core.DeleteAction).GetName()
		t.Logf("deleted pod %s", name)
		return true, nil, errors.NewNotFound(schema.GroupResource{Group: "", Resource: "pods"}, "nginx")
	})

	c.client.AddReactor("get", "pods", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		name := action.(core.GetAction).GetName()
		t.Logf("get pod %s", name)
		return true, nil, errors.NewNotFound(schema.GroupResource{Group: "", Resource: "pods"}, "nginx")
	})

	err = c.deletePodsFromKubernetesHandler(ctx, key)
	if err != nil {
		t.Error("Failed")
	}
	_, err = c.client.CoreV1().Pods(podCopy.Namespace).Get(ctx, podCopy.Name, v1.GetOptions{})
	if err == nil {
		t.Log("delete success")
		return
	}
	if !errors.IsNotFound(err) {
		t.Fatal("Desired pod not exist")
	}
	t.Log("pod not found test success")

	// test uid not equal before query
	c = newTestController()
	fk8s = &fake.Clientset{}
	c.client = fk8s
	c.knownPods.Store(key, &knownPod{lastPodStatusReceivedFromProvider: podCopy})
	c.deletePodsFromKubernetes.Enqueue(ctx, key)
	// add new pod
	if err = c.podsInformer.Informer().GetStore().Add(podCopy); err != nil {
		t.Fatal(err)
	}
	c.client.AddReactor("delete", "pods", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		name := action.(core.DeleteAction).GetName()
		t.Logf("deleted pod %s", name)
		return true, nil, errors.NewNotFound(schema.GroupResource{Group: "", Resource: "pods"}, "nginx")
	})

	c.client.AddReactor("get", "pods", func(action core.Action) (handled bool, ret runtime.Object, err error) {
		name := action.(core.GetAction).GetName()
		t.Logf("get pod %s", name)
		return true, nil, errors.NewNotFound(schema.GroupResource{Group: "", Resource: "pods"}, "nginx")
	})

	err = c.deletePodsFromKubernetesHandler(ctx, key)
	if err != nil {
		t.Error("Failed")
	}
	_, err = c.client.CoreV1().Pods(podCopy.Namespace).Get(ctx, podCopy.Name, v1.GetOptions{})
	if err == nil {
		t.Log("delete success")
		return
	}
	if !errors.IsNotFound(err) {
		t.Fatal("Desired pod not exist")
	}
	t.Log("pod uid conflict test success")
}

func newPodSpec() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Name:  "nginx",
				Image: "nginx:1.15.12",
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 443,
						Protocol:      "tcp",
					},
				},
			},
		},
	}
}
