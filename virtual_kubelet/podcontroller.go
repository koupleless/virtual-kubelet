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
	"github.com/koupleless/virtual-kubelet/common/utils"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/koupleless/virtual-kubelet/common/errdefs"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/queue"
	"github.com/koupleless/virtual-kubelet/common/trace"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
)

// PodLifecycleHandler defines the interface used by the PodController to react
// to new and changed pods scheduled to the node that is being managed.
//
// Errors produced by these methods should implement an interface from
// github.com/virtual-kubelet/virtual-kubelet/errdefs package in order for the
// core logic to be able to understand the type of failure.
type PodLifecycleHandler interface {
	// CreatePod takes a Kubernetes Pod and deploys it within the provider.
	CreatePod(ctx context.Context, pod *corev1.Pod) error

	// UpdatePod takes a Kubernetes Pod and updates it within the provider.
	UpdatePod(ctx context.Context, pod *corev1.Pod) error

	// DeletePod takes a Kubernetes Pod and deletes it from the provider. Once a pod is deleted, the provider is
	// expected to call the NotifyPods callback with a terminal pod status where all the containers are in a terminal
	// state, as well as the pod. DeletePod may be called multiple times for the same pod.
	DeletePod(ctx context.Context, pod *corev1.Pod) error

	// GetPod retrieves a pod by name from the provider (can be cached).
	// The Pod returned is expected to be immutable, and may be accessed
	// concurrently outside of the calling goroutine. Therefore it is recommended
	// to return a version after DeepCopy.
	GetPod(ctx context.Context, namespace, name string) (*corev1.Pod, error)

	// GetPodStatus retrieves the status of a pod by name from the provider.
	// The PodStatus returned is expected to be immutable, and may be accessed
	// concurrently outside of the calling goroutine. Therefore it is recommended
	// to return a version after DeepCopy.
	GetPodStatus(ctx context.Context, namespace, name string) (*corev1.PodStatus, error)

	// GetPods retrieves a list of all pods running on the provider (can be cached).
	// The Pods returned are expected to be immutable, and may be accessed
	// concurrently outside of the calling goroutine. Therefore it is recommended
	// to return a version after DeepCopy.
	GetPods(context.Context) ([]*corev1.Pod, error)
}

// PodNotifier is used as an extension to PodLifecycleHandler to support async updates of pod statuses.
type PodNotifier interface {
	// NotifyPods instructs the notifier to call the passed in function when
	// the pod status changes. It should be called when a pod's status changes.
	//
	// The provided pointer to a Pod is guaranteed to be used in a read-only
	// fashion. The provided pod's PodStatus should be up to date when
	// this function is called.
	//
	// NotifyPods must not block the caller since it is only used to register the callback.
	// The callback passed into `NotifyPods` may block when called.
	NotifyPods(context.Context, func(*corev1.Pod))
}

// PodEventFilterFunc is used to filter pod events received from Kubernetes.
//
// Filters that return true means the event handler will be run
// Filters that return false means the filter will *not* be run.
type PodEventFilterFunc func(context.Context, *corev1.Pod) bool

// PodController is the controller implementation for Pod resources.
type PodController struct {
	provider PodLifecycleHandler

	// recorder is an event recorder for recording Event resources to the Kubernetes API.
	recorder record.EventRecorder

	client client.Client
	cache  cache.Cache

	syncPodsFromKubernetes *queue.Queue

	// deletePodsFromKubernetes is a queue on which pods are reconciled, and we check if pods are in API server after
	// the grace period
	deletePodsFromKubernetes *queue.Queue

	syncPodStatusFromProvider *queue.Queue

	// From the time of creation, to termination the knownPods map will contain the pods key
	// (derived from Kubernetes' cache library) -> a *knownPod struct.
	knownPods sync.Map

	// ready is a channel which will be closed once the pod controller is fully up and running.
	// this channel will never be closed if there is an error on startup.
	ready chan struct{}
	// done is closed when Run returns
	// Once done is closed `err` may be set to a non-nil value
	done chan struct{}

	mu sync.Mutex
	// err is set if there is an error while while running the pod controller.
	// Typically this would be errors that occur during startup.
	// Once err is set, `Run` should return.
	//
	// This is used since `pc.Run()` is typically called in a goroutine and managing
	// this can be non-trivial for callers.
	err error
}

type knownPod struct {
	// You cannot read (or modify) the fields in this struct without taking the lock. The individual fields
	// should be immutable to avoid having to hold the lock the entire time you're working with them
	sync.Mutex
	lastPodStatusReceivedFromProvider *corev1.Pod
	lastPodUsed                       *corev1.Pod
	lastPodStatusUpdateSkipped        bool
}

// PodControllerConfig is used to configure a new PodController.
type PodControllerConfig struct {
	Client client.Client
	Cache  cache.Cache

	EventRecorder record.EventRecorder

	Provider PodLifecycleHandler

	// SyncPodsFromKubernetesRateLimiter defines the rate limit for the SyncPodsFromKubernetes queue
	SyncPodsFromKubernetesRateLimiter workqueue.RateLimiter
	// SyncPodsFromKubernetesShouldRetryFunc allows for a custom retry policy for the SyncPodsFromKubernetes queue
	SyncPodsFromKubernetesShouldRetryFunc queue.ShouldRetryFunc

	// DeletePodsFromKubernetesRateLimiter defines the rate limit for the DeletePodsFromKubernetesRateLimiter queue
	DeletePodsFromKubernetesRateLimiter workqueue.RateLimiter
	// DeletePodsFromKubernetesShouldRetryFunc allows for a custom retry policy for the SyncPodsFromKubernetes queue
	DeletePodsFromKubernetesShouldRetryFunc queue.ShouldRetryFunc

	// SyncPodStatusFromProviderRateLimiter defines the rate limit for the SyncPodStatusFromProviderRateLimiter queue
	SyncPodStatusFromProviderRateLimiter workqueue.RateLimiter
	// SyncPodStatusFromProviderShouldRetryFunc allows for a custom retry policy for the SyncPodStatusFromProvider queue
	SyncPodStatusFromProviderShouldRetryFunc queue.ShouldRetryFunc
}

// NewPodController creates a new pod controller with the provided config.
func NewPodController(cfg PodControllerConfig) (*PodController, error) {
	if cfg.EventRecorder == nil {
		return nil, errdefs.InvalidInput("missing event recorder")
	}
	if cfg.Provider == nil {
		return nil, errdefs.InvalidInput("missing provider")
	}
	if cfg.Client == nil {
		return nil, errdefs.InvalidInput("missing client")
	}
	if cfg.Cache == nil {
		return nil, errdefs.InvalidInput("missing cache")
	}
	if cfg.SyncPodsFromKubernetesRateLimiter == nil {
		cfg.SyncPodsFromKubernetesRateLimiter = workqueue.DefaultControllerRateLimiter()
	}
	if cfg.DeletePodsFromKubernetesRateLimiter == nil {
		cfg.DeletePodsFromKubernetesRateLimiter = workqueue.DefaultControllerRateLimiter()
	}
	if cfg.SyncPodStatusFromProviderRateLimiter == nil {
		cfg.SyncPodStatusFromProviderRateLimiter = workqueue.DefaultControllerRateLimiter()
	}

	pc := &PodController{
		client:   cfg.Client,
		cache:    cfg.Cache,
		provider: cfg.Provider,
		ready:    make(chan struct{}),
		done:     make(chan struct{}),
		recorder: cfg.EventRecorder,
	}

	pc.syncPodsFromKubernetes = queue.New(cfg.SyncPodsFromKubernetesRateLimiter, "syncPodsFromKubernetes", pc.syncPodFromKubernetesHandler, cfg.SyncPodsFromKubernetesShouldRetryFunc)
	pc.deletePodsFromKubernetes = queue.New(cfg.DeletePodsFromKubernetesRateLimiter, "deletePodsFromKubernetes", pc.deletePodsFromKubernetesHandler, cfg.DeletePodsFromKubernetesShouldRetryFunc)
	pc.syncPodStatusFromProvider = queue.New(cfg.SyncPodStatusFromProviderRateLimiter, "syncPodStatusFromProvider", pc.syncPodStatusFromProviderHandler, cfg.SyncPodStatusFromProviderShouldRetryFunc)

	return pc, nil
}

type asyncProvider interface {
	PodLifecycleHandler
	PodNotifier
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers.  It will block until the
// context is cancelled, at which point it will shutdown the work queue and
// wait for workers to finish processing their current work items prior to
// returning.
//
// Once this returns, you should not re-use the controller.
func (pc *PodController) Run(ctx context.Context, podSyncWorkers int) (retErr error) {
	// Shutdowns are idempotent, so we can call it multiple times. This is in case we have to bail out early for some reason.
	// This is to make extra sure that any workers we started are terminated on exit
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	defer func() {
		pc.mu.Lock()
		pc.err = retErr
		close(pc.done)
		pc.mu.Unlock()
	}()

	var provider asyncProvider
	runProvider := func(context.Context) {}

	if p, ok := pc.provider.(asyncProvider); ok {
		provider = p
	}
	pc.provider = provider

	provider.NotifyPods(ctx, func(pod *corev1.Pod) {
		pc.enqueuePodStatusUpdate(ctx, pod.DeepCopy())
	})
	go runProvider(ctx)

	// Perform a reconciliation step that deletes any dangling pods from the provider.
	// This happens only when the virtual-kubelet is starting, and operates on a "best-effort" basis.
	// If by any reason the provider fails to delete a dangling pod, it will stay in the provider and deletion won't be retried.
	pc.deleteDanglingPods(ctx, podSyncWorkers)

	log.G(ctx).Info("starting workers")
	group := &wait.Group{}
	group.StartWithContext(ctx, func(ctx context.Context) {
		pc.syncPodsFromKubernetes.Run(ctx, podSyncWorkers)
	})
	group.StartWithContext(ctx, func(ctx context.Context) {
		pc.deletePodsFromKubernetes.Run(ctx, podSyncWorkers)
	})
	group.StartWithContext(ctx, func(ctx context.Context) {
		pc.syncPodStatusFromProvider.Run(ctx, podSyncWorkers)
	})
	defer group.Wait()
	log.G(ctx).Info("started workers")
	close(pc.ready)

	<-ctx.Done()
	log.G(ctx).Info("shutting down workers")

	return nil
}

// Ready returns a channel which gets closed once the PodController is ready to handle scheduled pods.
// This channel will never close if there is an error on startup.
// The status of this channel after shutdown is indeterminate.
func (pc *PodController) Ready() <-chan struct{} {
	return pc.ready
}

// Done returns a channel receiver which is closed when the pod controller has exited.
// Once the pod controller has exited you can call `pc.Err()` to see if any error occurred.
func (pc *PodController) Done() <-chan struct{} {
	return pc.done
}

// Err returns any error that has occurred and caused the pod controller to exit.
func (pc *PodController) Err() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.err
}

func (pc *PodController) StorePod(key string, pod *corev1.Pod) {
	podCopy := pod.DeepCopy()
	pc.knownPods.Store(key, &knownPod{
		lastPodStatusReceivedFromProvider: podCopy,
		lastPodStatusUpdateSkipped:        false,
	})
}

func (pc *PodController) LoadPod(key string) (any, bool) {
	return pc.knownPods.Load(key)
}

func (pc *PodController) DeletePod(key string) {
	pc.knownPods.Delete(key)
}

func (pc *PodController) CheckAndUpdatePod(ctx context.Context, key string, obj interface{}, newPod *corev1.Pod) {
	kPod := obj.(*knownPod)
	kPod.Lock()
	if kPod.lastPodStatusUpdateSkipped &&
		(!cmp.Equal(newPod.Status, kPod.lastPodStatusReceivedFromProvider.Status) ||
			!cmp.Equal(newPod.Annotations, kPod.lastPodStatusReceivedFromProvider.Annotations) ||
			!cmp.Equal(newPod.Labels, kPod.lastPodStatusReceivedFromProvider.Labels) ||
			!cmp.Equal(newPod.Finalizers, kPod.lastPodStatusReceivedFromProvider.Finalizers)) {

		kPod.lastPodStatusReceivedFromProvider = newPod
		// Reset this to avoid re-adding it continuously
		kPod.lastPodStatusUpdateSkipped = false
		pc.syncPodStatusFromProvider.Enqueue(ctx, key)
	}
	kPod.Unlock()
}

func (pc *PodController) SyncPodsFromKubernetesEnqueue(ctx context.Context, key string) {
	pc.syncPodsFromKubernetes.Enqueue(ctx, key)
}

func (pc *PodController) DeletePodsFromKubernetesForget(ctx context.Context, key string) {
	pc.deletePodsFromKubernetes.Forget(ctx, key)
}

func (pc *PodController) DeletePodsFromKubernetesEnqueue(ctx context.Context, key string) {
	pc.deletePodsFromKubernetes.Enqueue(ctx, key)
}

func (pc *PodController) SyncPodStatusFromProviderEnqueue(ctx context.Context, key string) {
	pc.syncPodStatusFromProvider.Enqueue(ctx, key)
}

// syncPodFromKubernetesHandler compares the actual state with the desired, and attempts to converge the two.
func (pc *PodController) syncPodFromKubernetesHandler(ctx context.Context, key string) error {
	ctx, span := trace.StartSpan(ctx, "syncPodFromKubernetesHandler")
	defer span.End()

	// Add the current key as an attribute to the current span.
	ctx = span.WithField(ctx, "key", key)
	log.G(ctx).WithField("key", key).Debug("sync handled")

	// Convert the namespace/name string into a distinct namespace and name.
	namespace, name, err := utils.SplitMetaNamespaceKey(key)
	if err != nil {
		// Log the error as a warning, but do not requeue the key as it is invalid.
		log.G(ctx).Warn(pkgerrors.Wrapf(err, "invalid resource key: %q", key))
		return nil
	}

	// Get the Pod resource with this namespace/name.
	pod := &corev1.Pod{}
	err = pc.cache.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      name,
	}, pod, &client.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			// We've failed to fetch the pod from the lister, but the error is not a 404.
			// Hence, we add the key back to the work queue so we can retry processing it later.
			err := pkgerrors.Wrapf(err, "failed to fetch pod with key %q from lister", key)
			span.SetStatus(err)
			return err
		}

		pod, err = pc.provider.GetPod(ctx, namespace, name)
		if err != nil && !errors.IsNotFound(err) {
			err = pkgerrors.Wrapf(err, "failed to fetch pod with key %q from provider", key)
			span.SetStatus(err)
			return err
		}
		if errors.IsNotFound(err) || pod == nil {
			return nil
		}

		err = pc.provider.DeletePod(ctx, pod)
		if errors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			err = pkgerrors.Wrapf(err, "failed to delete pod %q in the provider", loggablePodNameFromCoordinates(namespace, name))
			span.SetStatus(err)
		}

		pc.DeletePod(key)

		return err

	}

	// At this point we know the Pod resource has either been created or updated (which includes being marked for deletion).
	return pc.syncPodInProvider(ctx, pod, key)
}

// syncPodInProvider tries and reconciles the state of a pod by comparing its Kubernetes representation and the provider's representation.
func (pc *PodController) syncPodInProvider(ctx context.Context, pod *corev1.Pod, key string) (retErr error) {
	ctx, span := trace.StartSpan(ctx, "syncPodInProvider")
	defer span.End()

	// Add the pod's attributes to the current span.
	ctx = addPodAttributes(ctx, span, pod)

	// If the pod('s containers) is no longer in a running state then we force-delete the pod from API server
	// more context is here: https://github.com/virtual-kubelet/virtual-kubelet/pull/760
	if pod.DeletionTimestamp != nil && !running(&pod.Status) {
		log.G(ctx).Debug("Force deleting pod from API Server as it is no longer running")
		key = fmt.Sprintf("%v/%v", key, pod.UID)
		pc.deletePodsFromKubernetes.EnqueueWithoutRateLimit(ctx, key)
		return nil
	}
	obj, ok := pc.knownPods.Load(key)
	if !ok {
		// That means the pod was deleted while we were working
		return nil
	}
	kPod := obj.(*knownPod)
	kPod.Lock()
	if kPod.lastPodUsed != nil && podsEffectivelyEqual(kPod.lastPodUsed, pod) {
		kPod.Unlock()
		return nil
	}
	kPod.Unlock()

	defer func() {
		if retErr == nil {
			kPod.Lock()
			defer kPod.Unlock()
			kPod.lastPodUsed = pod
		}
	}()

	// Check whether the pod has been marked for deletion.
	// If it does, guarantee it is deleted in the provider and Kubernetes.
	if pod.DeletionTimestamp != nil {
		log.G(ctx).Debug("Deleting pod in provider")
		if err := pc.deletePod(ctx, pod); errors.IsNotFound(err) {
			log.G(ctx).Debug("Pod not found in provider")
		} else if err != nil {
			err := pkgerrors.Wrapf(err, "failed to delete pod %q in the provider", loggablePodName(pod))
			span.SetStatus(err)
			return err
		}

		key = fmt.Sprintf("%v/%v", key, pod.UID)
		pc.deletePodsFromKubernetes.EnqueueWithoutRateLimitWithDelay(ctx, key, time.Second*time.Duration(*pod.DeletionGracePeriodSeconds))
		return nil
	}

	// Ignore the pod if it is in the "Failed" or "Succeeded" state.
	if pod.Status.Phase == corev1.PodFailed || pod.Status.Phase == corev1.PodSucceeded {
		log.G(ctx).Warnf("skipping sync of pod %q in %q phase", loggablePodName(pod), pod.Status.Phase)
		return nil
	}

	// Create or update the pod in the provider.
	if err := pc.createOrUpdatePod(ctx, pod); err != nil {
		err := pkgerrors.Wrapf(err, "failed to sync pod %q in the provider", loggablePodName(pod))
		span.SetStatus(err)
		return err
	}
	return nil
}

// deleteDanglingPods checks whether the provider knows about any pods which Kubernetes doesn't know about, and deletes them.
func (pc *PodController) deleteDanglingPods(ctx context.Context, threadiness int) {
	ctx, span := trace.StartSpan(ctx, "deleteDanglingPods")
	defer span.End()

	// Grab the list of pods known to the provider.
	pps, err := pc.provider.GetPods(ctx)
	if err != nil {
		err := pkgerrors.Wrap(err, "failed to fetch the list of pods from the provider")
		span.SetStatus(err)
		log.G(ctx).Error(err)
		return
	}

	// Create a slice to hold the pods we will be deleting from the provider.
	ptd := make([]*corev1.Pod, 0)

	// Iterate over the pods known to the provider, marking for deletion those that don't exist in Kubernetes.
	// Take on this opportunity to populate the list of key that correspond to pods known to the provider.
	for _, pp := range pps {
		pod := &corev1.Pod{}
		err = pc.cache.Get(ctx, types.NamespacedName{
			Namespace: pp.Namespace,
			Name:      pp.Name,
		}, pod, &client.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				// The current pod does not exist in Kubernetes, so we mark it for deletion.
				ptd = append(ptd, pp)
				continue
			}
			// For some reason we couldn't fetch the pod from the lister, so we propagate the error.
			err := pkgerrors.Wrap(err, "failed to fetch pod from the lister")
			span.SetStatus(err)
			log.G(ctx).Error(err)
			return
		}
	}

	// We delete each pod in its own goroutine, allowing a maximum of "threadiness" concurrent deletions.
	semaphore := make(chan struct{}, threadiness)
	var wg sync.WaitGroup
	wg.Add(len(ptd))

	// Iterate over the slice of pods to be deleted and delete them in the provider.
	for _, pod := range ptd {
		go func(ctx context.Context, pod *corev1.Pod) {
			defer wg.Done()

			ctx, span := trace.StartSpan(ctx, "deleteDanglingPod")
			defer span.End()

			semaphore <- struct{}{}
			defer func() {
				<-semaphore
			}()

			// Add the pod's attributes to the current span.
			ctx = addPodAttributes(ctx, span, pod)
			// Actually delete the pod.
			if err := pc.provider.DeletePod(ctx, pod.DeepCopy()); err != nil && !errors.IsNotFound(err) {
				span.SetStatus(err)
				log.G(ctx).Errorf("failed to delete pod %q in provider", loggablePodName(pod))
			} else {
				log.G(ctx).Infof("deleted leaked pod %q in provider", loggablePodName(pod))
			}
		}(ctx, pod)
	}

	// Wait for all pods to be deleted.
	wg.Wait()
}

// loggablePodName returns the "namespace/name" key for the specified pod.
// If the key cannot be computed, "(unknown)" is returned.
// This method is meant to be used for logging purposes only.
func loggablePodName(pod *corev1.Pod) string {
	return utils.GetPodKey(pod)
}

// loggablePodNameFromCoordinates returns the "namespace/name" key for the pod identified by the specified namespace and name (coordinates).
func loggablePodNameFromCoordinates(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

// podsEffectivelyEqual compares two pods, and ignores the pod status, and the resource version
func podsEffectivelyEqual(p1, p2 *corev1.Pod) bool {
	filterForResourceVersion := func(p cmp.Path) bool {
		if p.String() == "ObjectMeta.ResourceVersion" {
			return true
		}
		if p.String() == "Status" {
			return true
		}
		return false
	}

	return cmp.Equal(p1, p2, cmp.FilterPath(filterForResourceVersion, cmp.Ignore()))
}

// borrowed from https://github.com/kubernetes/kubernetes/blob/f64c631cd7aea58d2552ae2038c1225067d30dde/pkg/kubelet/kubelet_pods.go#L944-L953
// running returns true, unless if every status is terminated or waiting, or the status list
// is empty.
func running(podStatus *corev1.PodStatus) bool {
	statuses := podStatus.ContainerStatuses
	for _, status := range statuses {
		if status.State.Terminated == nil && status.State.Waiting == nil {
			return true
		}
	}
	return false
}
