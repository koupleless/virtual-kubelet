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

package base_pod

import (
	"context"
	"fmt"
	"github.com/koupleless/module-controller/common/queue"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	"github.com/virtual-kubelet/virtual-kubelet/trace"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	corev1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"os"
	"sync"
	"time"
)

type BasePodController struct {
	basePodInformerFactory informers.SharedInformerFactory
	// basePodsInformer is an informer for Pod resources.
	basePodsInformer corev1informers.PodInformer
	// basePodsLister is able to list/get Pod resources from a shared informer's store.
	basePodsLister corev1listers.PodLister

	vNodeInformerFactory informers.SharedInformerFactory
	// vNodeInformer is an informer for Node resources.
	vNodeInformer corev1informers.NodeInformer
	// vNodeLister is able to list/get Node resources from a shared informer's store.
	vNodeLister corev1listers.NodeLister

	vpodInformerFactory informers.SharedInformerFactory
	// vPodInformer is an informer for Pod resources.
	vPodInformer corev1informers.PodInformer
	// vPodLister is able to list/get Pod resources from a shared informer's store.
	vPodLister corev1listers.PodLister

	vNodeClient   corev1client.NodesGetter
	vPodClient    corev1client.PodsGetter
	basePodClient corev1client.PodsGetter

	basePodInfo *basePod
	vNodeInfo   *vnode
	knownVPods  sync.Map

	basePodOperationQueue       *queue.Queue
	vPodOperationQueue          *queue.Queue
	syncPodStatusFromProvider   *queue.Queue
	syncVNodeStatusFromProvider *queue.Queue

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

type basePod struct {
	// You cannot read (or modify) the fields in this struct without taking the lock. The individual fields
	// should be immutable to avoid having to hold the lock the entire time you're working with them
	sync.Mutex
	lastPodStatusReceivedFromProvider *corev1.Pod
	deleting                          bool
}

type vPod struct {
	// You cannot read (or modify) the fields in this struct without taking the lock. The individual fields
	// should be immutable to avoid having to hold the lock the entire time you're working with them
	sync.Mutex
	lastPodStatusReceivedFromProvider *corev1.Pod
}

type vnode struct {
	// You cannot read (or modify) the fields in this struct without taking the lock. The individual fields
	// should be immutable to avoid having to hold the lock the entire time you're working with them
	sync.Mutex
	lastNodeStatusReceivedFromKubernetes *corev1.Node
	evicting                             bool
}

type ShouldRetryFunc = queue.ShouldRetryFunc

type BasePodControllerConfig struct {
	BasePodKubeConfigPath string
	VNodeName             string

	// VirtualClientSet is used to perform actions on the vnode belonging k8s API, such as updating pod status
	// This field is required
	VirtualClientSet kubernetes.Interface
}

func (pc *BasePodController) syncBasePodStatusFromProviderHandler(ctx context.Context, key string) (retErr error) {
	ctx, span := trace.StartSpan(ctx, "syncBasePodStatusFromProviderHandler")
	defer span.End()

	ctx = span.WithField(ctx, "key", key)
	log.G(ctx).Debug("processing base pod status update")
	defer func() {
		span.SetStatus(retErr)
		if retErr != nil {
			log.G(ctx).WithError(retErr).Error("Error processing base pod status update")
		}
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return errors.Wrap(err, "error splitting cache key")
	}

	pod, err := pc.basePodsLister.Pods(namespace).Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.G(ctx).WithError(err).Debug("Skipping pod status update for pod missing in Kubernetes")
			return nil
		}
		return errors.Wrap(err, "error looking up pod")
	}

	return pc.updateBasePodStatus(ctx, pod, key)
}

func (pc *BasePodController) syncVNodeStatusFromProviderHandler(ctx context.Context, key string) (retErr error) {
	ctx, span := trace.StartSpan(ctx, "syncVNodeStatusFromProviderHandler")
	defer span.End()

	ctx = span.WithField(ctx, "key", key)
	log.G(ctx).Debug("processing vnode status update")
	defer func() {
		span.SetStatus(retErr)
		if retErr != nil {
			log.G(ctx).WithError(retErr).Error("Error processing vnode status update")
		}
	}()

	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return errors.Wrap(err, "error splitting cache key")
	}

	node, err := pc.vNodeLister.Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.G(ctx).WithError(err).Debug("Skipping vnode status update for vnode missing in Kubernetes")
			return nil
		}
		return errors.Wrap(err, "error looking up vnode")
	}

	return pc.updateVNodeStatus(ctx, node)
}

func (pc *BasePodController) updateBasePodStatus(ctx context.Context, podFromKubernetes *corev1.Pod, key string) error {
	if shouldSkipPodStatusUpdate(podFromKubernetes) {
		return nil
	}

	ctx, span := trace.StartSpan(ctx, "updateBasePodStatus")
	defer span.End()
	ctx = addPodAttributes(ctx, span, podFromKubernetes)

	pc.basePodInfo.Lock()
	pc.basePodInfo.lastPodStatusReceivedFromProvider = podFromKubernetes
	pc.basePodInfo.Unlock()

	if podFromKubernetes.DeletionTimestamp != nil {
		// in deletion period
		pc.basePodOperationQueue.Enqueue(ctx, key)
	}

	log.G(ctx).WithFields(log.Fields{
		"new reason": podFromKubernetes.Status.Reason,
	}).Debug("Updated pod status in kubernetes")

	return nil
}

func (pc *BasePodController) vPodOperationHandler(ctx context.Context, key string) (retErr error) {
	ctx, span := trace.StartSpan(ctx, "vPodOperationHandler")
	defer span.End()

	ctx = span.WithField(ctx, "key", key)
	log.G(ctx).Debug("processing vpod status update")
	defer func() {
		span.SetStatus(retErr)
		if retErr != nil {
			log.G(ctx).WithError(retErr).Error("Error processing vpod status update")
		}
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return errors.Wrap(err, "error splitting cache key")
	}
	moduleFinalizerKey := formatModuleFinalizerKey(name)

	pod, err := pc.vPodLister.Pods(namespace).Get(name)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			// We've failed to fetch the pod from the lister, but the error is not a 404.
			err = errors.Wrapf(err, "failed to fetch vpod with key %q from lister", key)
			span.SetStatus(err)
			return err
		}

		pc.knownVPods.Delete(key)
		// remove base pod finalizer tag
		pc.basePodInfo.Lock()
		indexOfFinalizers := -1
		for index, finalizer := range pc.basePodInfo.lastPodStatusReceivedFromProvider.Finalizers {
			if finalizer == moduleFinalizerKey {
				indexOfFinalizers = index
				break
			}
		}
		if indexOfFinalizers != -1 {
			// update local finalizers
			pc.basePodInfo.lastPodStatusReceivedFromProvider.Finalizers = append(pc.basePodInfo.lastPodStatusReceivedFromProvider.Finalizers[:indexOfFinalizers], pc.basePodInfo.lastPodStatusReceivedFromProvider.Finalizers[indexOfFinalizers+1:]...)
			// patch the newest finalizers to base pod
			patchOps := PatchOps{
				{
					OP:    PatchOpTypeRemove,
					Path:  fmt.Sprintf("/metadata/finalizers/%d", indexOfFinalizers),
					Value: nil,
				},
			}

			resultPod, err := pc.basePodClient.Pods(pc.basePodInfo.lastPodStatusReceivedFromProvider.Namespace).Patch(ctx, pc.basePodInfo.lastPodStatusReceivedFromProvider.Name, types.JSONPatchType, patchOps.bytes(), metav1.PatchOptions{})
			if err != nil {
				log.G(ctx).WithError(err).Error("Error deleting base pod finalizers")
			}
			pc.basePodInfo.lastPodStatusReceivedFromProvider = resultPod.DeepCopy()
		}
		pc.basePodInfo.Unlock()
	} else {
		_, ok := pc.knownVPods.Load(key)
		if !ok {
			pc.knownVPods.Store(key, &vPod{
				Mutex:                             sync.Mutex{},
				lastPodStatusReceivedFromProvider: pod,
			})
			// add a new pod, need to patch a new finalizer to base pod
			pc.basePodInfo.Lock()
			// check if there is no finalizer now, use add op to replace the finalizers
			initEmpty := false
			if len(pc.basePodInfo.lastPodStatusReceivedFromProvider.Finalizers) == 0 {
				initEmpty = true
			}
			pc.basePodInfo.lastPodStatusReceivedFromProvider.Finalizers = append(pc.basePodInfo.lastPodStatusReceivedFromProvider.Finalizers, moduleFinalizerKey)
			patchOps := PatchOps{
				{
					OP: PatchOpTypeAdd,
				},
			}
			if initEmpty {
				patchOps[0].Path = "/metadata/finalizers"
				patchOps[0].Value = pc.basePodInfo.lastPodStatusReceivedFromProvider.Finalizers
			} else {
				patchOps[0].Path = "/metadata/finalizers/-"
				patchOps[0].Value = moduleFinalizerKey
			}
			bytes := patchOps.bytes()
			resultPod, err := pc.basePodClient.Pods(pc.basePodInfo.lastPodStatusReceivedFromProvider.Namespace).Patch(ctx, pc.basePodInfo.lastPodStatusReceivedFromProvider.Name, types.JSONPatchType, bytes, metav1.PatchOptions{})
			if err != nil {
				log.G(ctx).WithError(err).Error("Error adding base pod finalizers")
			}
			log.G(ctx).WithFields(log.Fields{
				"finalizer": moduleFinalizerKey,
				"ops":       string(bytes),
			}).Info("adding base pod finalizers succeed")
			pc.basePodInfo.lastPodStatusReceivedFromProvider = resultPod.DeepCopy()
			// if there are finalizers in the array, use add op to add the finalizer to the end
			pc.basePodInfo.Unlock()
		}
	}
	return nil
}

func (pc *BasePodController) updateVNodeStatus(ctx context.Context, nodeFromKubernetes *corev1.Node) error {
	ctx, span := trace.StartSpan(ctx, "updateVNodeStatus")
	defer span.End()
	ctx = addVNodeAttributes(ctx, span, nodeFromKubernetes)

	// sync local vNode status, for status updating
	pc.vNodeInfo.Lock()
	pc.vNodeInfo.lastNodeStatusReceivedFromKubernetes = nodeFromKubernetes
	// check contains module taint label
	containsModuleEvictionTaint := false
	for _, taint := range nodeFromKubernetes.Spec.Taints {
		if taint.Key == VNodeDeletionTaintKey {
			containsModuleEvictionTaint = true
			break
		}
	}
	pc.vNodeInfo.Unlock()
	if containsModuleEvictionTaint {
		// in base pod deletion process, delete all the pods known
		pc.knownVPods.Range(func(key, value any) bool {
			vpod := value.(*vPod)
			vpod.Lock()
			vpodCopy := vpod.lastPodStatusReceivedFromProvider.DeepCopy()
			vpod.Unlock()
			err := pc.vPodClient.Pods(vpodCopy.Namespace).EvictV1beta1(ctx, &policyv1beta1.Eviction{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vpodCopy.Name,
					Namespace: vpodCopy.Namespace,
				},
				DeleteOptions: &metav1.DeleteOptions{},
			})
			if err != nil {
				log.G(ctx).WithError(err).Error("Error deleting vpod")
			}
			return true
		})
	}

	return nil
}

func (pc *BasePodController) basePodOperationHandler(ctx context.Context, key string) (retErr error) {
	ctx, span := trace.StartSpan(ctx, "basePodOperationHandler")
	defer span.End()

	ctx = span.WithField(ctx, "key", key)
	log.G(ctx).Debug("processing base pod delete operation")
	defer func() {
		span.SetStatus(retErr)
		if retErr != nil {
			log.G(ctx).WithError(retErr).Error("Error processing base pod delete operation")
		}
	}()

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return errors.Wrap(err, "error splitting cache key")
	}

	pod, err := pc.basePodsLister.Pods(namespace).Get(name)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.G(ctx).WithError(err).Debug("Skipping base pod operation for base pod missing in Kubernetes")
			return nil
		}
		return errors.Wrap(err, "error looking up base pod")
	}

	return pc.processPodDeletion(ctx, pod)
}

func (pc *BasePodController) processPodDeletion(ctx context.Context, podFromKubernetes *corev1.Pod) error {
	if pc.shouldSkipPodDeletionProcess() {
		return nil
	}

	ctx, span := trace.StartSpan(ctx, "basePodDeletion")
	defer span.End()
	ctx = addPodAttributes(ctx, span, podFromKubernetes)
	// base pod may delete by some reason, e.g. rolling update of deployment or being evicted by node
	// we need to catch and start vnode deletion process
	// step into deletion period, set vnode eviction condition (taints)
	pc.vNodeInfo.Lock()
	currVNodeStatus := pc.vNodeInfo.lastNodeStatusReceivedFromKubernetes.DeepCopy()
	pc.vNodeInfo.Unlock()
	if podFromKubernetes.DeletionTimestamp != nil {
		deletionTaint := corev1.Taint{
			Key:    VNodeDeletionTaintKey,
			Effect: corev1.TaintEffectNoExecute,
			Value:  "True",
		}
		currVNodeStatus.Spec.Taints = append(currVNodeStatus.Spec.Taints, deletionTaint)

		patchOps := PatchOps{
			{
				OP:    PatchOpTypeAdd,
				Path:  "/spec/taints/-",
				Value: deletionTaint,
			},
		}

		resultVNode, err := pc.vNodeClient.Nodes().Patch(ctx, currVNodeStatus.Name, types.JSONPatchType, patchOps.bytes(), metav1.PatchOptions{})
		if err != nil {
			log.G(ctx).WithError(err).Error("Error deleting base pod finalizers")
		}
		pc.vNodeInfo.lastNodeStatusReceivedFromKubernetes = resultVNode.DeepCopy()
	} else {
		// check if there is taint key in vnode, remove
		indexOfTaint := -1
		for index, taint := range currVNodeStatus.Spec.Taints {
			if taint.Key == VNodeDeletionTaintKey {
				indexOfTaint = index
			}
		}
		if indexOfTaint != -1 {
			currVNodeStatus.Spec.Taints = append(currVNodeStatus.Spec.Taints[:indexOfTaint], currVNodeStatus.Spec.Taints[indexOfTaint+1:]...)
		}
	}

	log.G(ctx).Debug("complete pod deletion process, start vnode pod eviction")

	return nil
}

func (pc *BasePodController) shouldSkipPodDeletionProcess() bool {
	// check if deletion has been processed
	pc.basePodInfo.Lock()
	if pc.basePodInfo.deleting {
		return true
	}
	pc.basePodInfo.deleting = true
	pc.basePodInfo.Unlock()
	// check if the base pod deletion process has started, vnode deletion taint has been set
	for _, taint := range pc.vNodeInfo.lastNodeStatusReceivedFromKubernetes.Spec.Taints {
		if taint.Key == VNodeDeletionTaintKey {
			return true
		}
	}
	return false
}

// Ready returns a channel which gets closed once the BasePodController is ready to handle scheduled pods.
// This channel will never close if there is an error on startup.
// The status of this channel after shutdown is indeterminate.
func (pc *BasePodController) Ready() <-chan struct{} {
	return pc.ready
}

// Done returns a channel receiver which is closed when the pod controller has exited.
// Once the pod controller has exited you can call `pc.Err()` to see if any error occurred.
func (pc *BasePodController) Done() <-chan struct{} {
	return pc.done
}

// Err returns any error that has occurred and caused the pod controller to exit.
func (pc *BasePodController) Err() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	return pc.err
}

// WaitReady waits for the specified timeout for the controller to be ready.
//
// The timeout is for convenience so the caller doesn't have to juggle an extra context.
func (pc *BasePodController) WaitReady(ctx context.Context, timeout time.Duration) error {
	if timeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	select {
	case <-pc.ready:
		return nil
	case <-pc.done:
		return fmt.Errorf("controller exited before ready: %w", pc.err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (pc *BasePodController) BasePodUpdateHandler(oldObj, newObj interface{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, span := trace.StartSpan(ctx, "UpdateBasePodFunc")
	defer span.End()

	// Create a copy of the old and new pod objects so we don't mutate the cache.
	newPod := newObj.(*corev1.Pod)

	// At this point we know that something in .metadata or .spec has changed, so we must proceed to sync the pod.
	// notice: there will be a request of update in every resync period
	if key, err := cache.MetaNamespaceKeyFunc(newPod); err != nil {
		log.G(ctx).Error(err)
	} else {
		// key: namespace/name
		ctx = span.WithField(ctx, "key", key)
		pc.syncPodStatusFromProvider.Enqueue(ctx, key)
	}
}

func (pc *BasePodController) VNodeAddHandler(newObj interface{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, span := trace.StartSpan(ctx, "AddVNodeFunc")
	defer span.End()

	// Create a copy of the old and new pod objects so we don't mutate the cache.
	newNode := newObj.(*corev1.Node)

	// At this point we know that something in .metadata or .spec has changed, so we must proceed to sync the pod.
	// notice: there will be a request of update in every resync period
	if key, err := cache.MetaNamespaceKeyFunc(newNode); err != nil {
		log.G(ctx).Error(err)
	} else {
		// key: namespace/name
		ctx = span.WithField(ctx, "key", key)
		pc.syncVNodeStatusFromProvider.Enqueue(ctx, key)
	}
}

func (pc *BasePodController) VNodeUpdateHandler(oldObj, newObj interface{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, span := trace.StartSpan(ctx, "UpdateVNodeFunc")
	defer span.End()

	// Create a copy of the old and new pod objects so we don't mutate the cache.
	newNode := newObj.(*corev1.Node)

	// At this point we know that something in .metadata or .spec has changed, so we must proceed to sync the pod.
	// notice: there will be a request of update in every resync period
	if key, err := cache.MetaNamespaceKeyFunc(newNode); err != nil {
		log.G(ctx).Error(err)
	} else {
		// key: namespace/name
		ctx = span.WithField(ctx, "key", key)
		pc.syncVNodeStatusFromProvider.Enqueue(ctx, key)
	}
}

func (pc *BasePodController) VPodAddHandler(obj interface{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, span := trace.StartSpan(ctx, "AddVPodFunc")
	defer span.End()

	newPod := obj.(*corev1.Pod)

	// At this point we know that something in .metadata or .spec has changed, so we must proceed to sync the pod.
	// notice: there will be a request of update in every resync period
	if key, err := cache.MetaNamespaceKeyFunc(newPod); err != nil {
		log.G(ctx).Error(err)
	} else {
		// key: namespace/name
		ctx = span.WithField(ctx, "key", key)
		pc.vPodOperationQueue.Enqueue(ctx, key)
	}
}

func (pc *BasePodController) VPodDeleteHandler(obj interface{}) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx, span := trace.StartSpan(ctx, "DeleteVPodFunc")
	defer span.End()

	oldPod := obj.(*corev1.Pod)

	// At this point we know that something in .metadata or .spec has changed, so we must proceed to sync the pod.
	// notice: there will be a request of update in every resync period
	if key, err := cache.MetaNamespaceKeyFunc(oldPod); err != nil {
		log.G(ctx).Error(err)
	} else {
		// key: namespace/name
		ctx = span.WithField(ctx, "key", key)
		pc.vPodOperationQueue.Enqueue(ctx, key)
	}
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers.  It will block until the
// context is cancelled, at which point it will shutdown the work queue and
// wait for workers to finish processing their current work items prior to
// returning.
//
// Once this returns, you should not re-use the controller.
func (pc *BasePodController) Run(ctx context.Context, podSyncWorkers int) (retErr error) {
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

	pc.basePodInformerFactory.Start(ctx.Done())
	pc.vNodeInformerFactory.Start(ctx.Done())
	pc.vpodInformerFactory.Start(ctx.Done())

	// Wait for the caches to be synced *before* starting to do work.
	if ok := cache.WaitForCacheSync(ctx.Done(), pc.basePodsInformer.Informer().HasSynced); !ok {
		return errors.New("failed to wait for caches to sync")
	}
	log.G(ctx).Info("Base Pod cache in-sync")

	if ok := cache.WaitForCacheSync(ctx.Done(), pc.vNodeInformer.Informer().HasSynced); !ok {
		return errors.New("failed to wait for caches to sync")
	}
	log.G(ctx).Info("vnode cache in-sync")

	if ok := cache.WaitForCacheSync(ctx.Done(), pc.vPodInformer.Informer().HasSynced); !ok {
		return errors.New("failed to wait for caches to sync")
	}
	log.G(ctx).Info("vpod cache in-sync")

	// init base pod information and vnode information
	podList, retErr := pc.basePodsLister.List(labels.Everything())
	if retErr != nil {
		return errors.Wrap(retErr, "failed to list base pods")
	}
	if len(podList) == 0 {
		return errors.New("no base pod found")
	}
	pc.basePodInfo.Lock()
	pc.basePodInfo.lastPodStatusReceivedFromProvider = podList[0]
	pc.basePodInfo.Unlock()

	// Set up event handlers for when Pod resources change. Since the pod cache is in-sync, the informer will generate
	// synthetic add events at this point. It again avoids the race condition of adding handlers while the cache is
	// syncing.

	var basePodEventHandler cache.ResourceEventHandler = cache.ResourceEventHandlerFuncs{
		UpdateFunc: pc.BasePodUpdateHandler,
	}

	var vNodeEventHandler cache.ResourceEventHandler = cache.ResourceEventHandlerFuncs{
		AddFunc:    pc.VNodeAddHandler,
		UpdateFunc: pc.VNodeUpdateHandler,
	}

	var vPodEventHandler cache.ResourceEventHandler = cache.ResourceEventHandlerFuncs{
		AddFunc:    pc.VPodAddHandler,
		DeleteFunc: pc.VPodDeleteHandler,
	}

	log.G(ctx).Info("starting workers")
	go pc.basePodOperationQueue.Run(ctx, podSyncWorkers)
	go pc.vPodOperationQueue.Run(ctx, podSyncWorkers)
	go pc.syncPodStatusFromProvider.Run(ctx, podSyncWorkers)
	go pc.syncVNodeStatusFromProvider.Run(ctx, podSyncWorkers)
	log.G(ctx).Info("started workers")

	_, err := pc.basePodsInformer.Informer().AddEventHandler(basePodEventHandler)
	if err != nil {
		log.G(ctx).Error(err)
	}

	_, err = pc.vNodeInformer.Informer().AddEventHandler(vNodeEventHandler)
	if err != nil {
		log.G(ctx).Error(err)
	}

	_, err = pc.vPodInformer.Informer().AddEventHandler(vPodEventHandler)
	if err != nil {
		log.G(ctx).Error(err)
	}

	close(pc.ready)

	<-ctx.Done()
	log.G(ctx).Info("shutting down workers")

	return nil
}

// BasePodNameInformerFilter is a filter that you should use when creating a base pod informer for use with the base pod controller.
func BasePodNameInformerFilter(pod string) informers.SharedInformerOption {
	return informers.WithTweakListOptions(func(options *metav1.ListOptions) {
		// filter the pod with base pod name
		options.FieldSelector = fields.OneTermEqualSelector("metadata.name", pod).String()
	})
}

// VNodeInformerFilter is a filter that you should use when creating a vnode informer for use with the vnode controller.
func VNodeInformerFilter(vNodeName string) informers.SharedInformerOption {
	return informers.WithTweakListOptions(func(options *metav1.ListOptions) {
		// filter the pod with base pod name
		options.FieldSelector = fields.OneTermEqualSelector("metadata.name", vNodeName).String()
	})
}

// VPodInformerFilter is a filter that you should use when creating a vpod informer for use with the vpod controller.
func VPodInformerFilter(vNodeName string) informers.SharedInformerOption {
	return informers.WithTweakListOptions(func(options *metav1.ListOptions) {
		// filter the pod with base pod name
		options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", vNodeName).String()
	})
}

func shouldSkipPodStatusUpdate(pod *corev1.Pod) bool {
	return pod.Status.Phase == corev1.PodSucceeded ||
		pod.Status.Phase == corev1.PodFailed
}

func addPodAttributes(ctx context.Context, span trace.Span, pod *corev1.Pod) context.Context {
	return span.WithFields(ctx, log.Fields{
		"uid":       string(pod.GetUID()),
		"namespace": pod.GetNamespace(),
		"name":      pod.GetName(),
		"phase":     string(pod.Status.Phase),
		"reason":    pod.Status.Reason,
	})
}

func addVNodeAttributes(ctx context.Context, span trace.Span, node *corev1.Node) context.Context {
	return span.WithFields(ctx, log.Fields{
		"uid":       string(node.GetUID()),
		"namespace": node.GetNamespace(),
		"name":      node.GetName(),
		"phase":     string(node.Status.Phase),
	})
}

// todo: more complicated retry logic
func defaultRetryFunc(ctx context.Context, key string, timesTried int, originallyAdded time.Time, err error) (*time.Duration, error) {
	duration := time.Millisecond * 100
	return &duration, nil
}

func formatModuleFinalizerKey(key string) string {
	return fmt.Sprintf(ModuleFinalizerPattern, key)
}

// NewBasePodController creates a new base pod controller with the provided config.
func NewBasePodController(cfg BasePodControllerConfig) (*BasePodController, error) {
	// load in cluster basePodClient set
	basePodName := os.Getenv("BASE_POD_NAME")
	basePodNamespace := os.Getenv("BASE_POD_NAMESPACE")
	// construct in cluster basePodClient set
	inClusterClientset, err := nodeutil.ClientsetFromEnv(cfg.BasePodKubeConfigPath)
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to in-cluster basePodClient")
	}
	basePodInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		inClusterClientset,
		time.Minute,
		informers.WithNamespace(basePodNamespace),
		BasePodNameInformerFilter(basePodName),
	)

	basePodInformer := basePodInformerFactory.Core().V1().Pods()

	vNodeInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		cfg.VirtualClientSet,
		time.Minute,
		VNodeInformerFilter(cfg.VNodeName),
	)

	vNodeInformer := vNodeInformerFactory.Core().V1().Nodes()

	vPodInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		cfg.VirtualClientSet,
		time.Minute,
		VPodInformerFilter(cfg.VNodeName),
	)

	vPodInformer := vPodInformerFactory.Core().V1().Pods()

	pc := &BasePodController{
		basePodClient:          inClusterClientset.CoreV1(),
		basePodInformerFactory: basePodInformerFactory,
		basePodsInformer:       basePodInformer,
		basePodsLister:         basePodInformer.Lister(),
		vNodeClient:            cfg.VirtualClientSet.CoreV1(),
		vNodeInformerFactory:   vNodeInformerFactory,
		vNodeLister:            vNodeInformer.Lister(),
		vNodeInformer:          vNodeInformer,
		vPodClient:             cfg.VirtualClientSet.CoreV1(),
		vpodInformerFactory:    vPodInformerFactory,
		vPodLister:             vPodInformer.Lister(),
		vPodInformer:           vPodInformer,
		basePodInfo:            &basePod{},
		vNodeInfo:              &vnode{},
		ready:                  make(chan struct{}),
		done:                   make(chan struct{}),
	}

	pc.basePodOperationQueue = queue.New(
		workqueue.DefaultControllerRateLimiter(),
		"basePodOperationQueue",
		pc.basePodOperationHandler,
		defaultRetryFunc)
	pc.vPodOperationQueue = queue.New(
		workqueue.DefaultControllerRateLimiter(),
		"vPodOperationQueue",
		pc.vPodOperationHandler,
		defaultRetryFunc)
	pc.syncPodStatusFromProvider = queue.New(
		workqueue.DefaultControllerRateLimiter(),
		"syncBasePodStatusOperationQueue",
		pc.syncBasePodStatusFromProviderHandler,
		defaultRetryFunc)
	pc.syncVNodeStatusFromProvider = queue.New(
		workqueue.DefaultControllerRateLimiter(),
		"syncVNodeStatusOperationQueue",
		pc.syncVNodeStatusFromProviderHandler,
		defaultRetryFunc)
	return pc, nil
}
