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

package node

import (
	"context"
	"fmt"
	"github.com/koupleless/virtual-kubelet/java/common"
	"strings"
	"time"

	"github.com/koupleless/virtual-kubelet/common/queue"

	"github.com/google/go-cmp/cmp"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/trace"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"
)

const (
	podStatusReasonProviderFailed = "ProviderFailed"
	podEventCreateFailed          = "ProviderCreateFailed"
	podEventCreateSuccess         = "ProviderCreateSuccess"
	podEventDeleteFailed          = "ProviderDeleteFailed"
	podEventDeleteSuccess         = "ProviderDeleteSuccess"
	podEventUpdateFailed          = "ProviderUpdateFailed"
	podEventUpdateSuccess         = "ProviderUpdateSuccess"

	// 151 milliseconds is just chosen as a small prime number to retry between
	// attempts to get a notification from the provider to VK
	notificationRetryPeriod = 151 * time.Millisecond
)

func addPodAttributes(ctx context.Context, span trace.Span, pod *corev1.Pod) context.Context {
	return span.WithFields(ctx, log.Fields{
		"uid":       string(pod.GetUID()),
		"namespace": pod.GetNamespace(),
		"name":      pod.GetName(),
		"phase":     string(pod.Status.Phase),
		"reason":    pod.Status.Reason,
	})
}

func (pc *PodController) createOrUpdatePod(ctx context.Context, pod *corev1.Pod) error {

	ctx, span := trace.StartSpan(ctx, "createOrUpdatePod")
	defer span.End()
	addPodAttributes(ctx, span, pod)

	ctx = span.WithFields(ctx, log.Fields{
		"pod":       pod.GetName(),
		"namespace": pod.GetNamespace(),
	})

	// We do this so we don't mutate the pod from the informer cache
	pod = pod.DeepCopy()

	// We have to use a  different pod that we pass to the provider than the one that gets used in handleProviderError
	// because the provider  may manipulate the pod in a separate goroutine while we were doing work
	podForProvider := pod.DeepCopy()

	// Check if the pod is already known by the provider.
	// NOTE: Some providers return a non-nil error in their GetPod implementation when the pod is not found while some other don't.
	// Hence, we ignore the error and just act upon the pod if it is non-nil (meaning that the provider still knows about the pod).
	if podFromProvider, _ := pc.provider.GetPod(ctx, pod.Namespace, pod.Name); podFromProvider != nil {
		if !common.PodsEqual(podFromProvider, podForProvider) {
			log.G(ctx).Debugf("Pod %s exists, updating pod in provider", podFromProvider.Name)
			if origErr := pc.provider.UpdatePod(ctx, podForProvider); origErr != nil {
				pc.handleProviderError(ctx, span, origErr, pod)
				pc.recorder.Event(pod, corev1.EventTypeWarning, podEventUpdateFailed, origErr.Error())

				return origErr
			}
			log.G(ctx).Info("Updated pod in provider")
			pc.recorder.Event(pod, corev1.EventTypeNormal, podEventUpdateSuccess, "Update pod in provider successfully")
		}
	} else {
		if origErr := pc.provider.CreatePod(ctx, podForProvider); origErr != nil {
			pc.handleProviderError(ctx, span, origErr, pod)
			pc.recorder.Event(pod, corev1.EventTypeWarning, podEventCreateFailed, origErr.Error())
			return origErr
		}
		log.G(ctx).Info("Created pod in provider")
		pc.recorder.Event(pod, corev1.EventTypeNormal, podEventCreateSuccess, "Create pod in provider successfully")
	}
	return nil
}

func (pc *PodController) handleProviderError(ctx context.Context, span trace.Span, origErr error, pod *corev1.Pod) {
	podPhase := corev1.PodPending
	if pod.Spec.RestartPolicy == corev1.RestartPolicyNever {
		podPhase = corev1.PodFailed
	}

	pod.ResourceVersion = "" // Blank out resource version to prevent object has been modified error
	pod.Status.Phase = podPhase
	pod.Status.Reason = podStatusReasonProviderFailed
	pod.Status.Message = origErr.Error()

	logger := log.G(ctx).WithFields(log.Fields{
		"podPhase": podPhase,
		"reason":   pod.Status.Reason,
	})

	_, err := pc.client.Pods(pod.Namespace).UpdateStatus(ctx, pod, metav1.UpdateOptions{})
	if err != nil {
		logger.WithError(err).Warn("Failed to update pod status")
	} else {
		logger.Info("Updated k8s pod status")
	}
	span.SetStatus(origErr)
}

func (pc *PodController) deletePod(ctx context.Context, pod *corev1.Pod) error {
	ctx, span := trace.StartSpan(ctx, "deletePod")
	defer span.End()
	ctx = addPodAttributes(ctx, span, pod)

	err := pc.provider.DeletePod(ctx, pod.DeepCopy())
	if err != nil {
		span.SetStatus(err)
		pc.recorder.Event(pod, corev1.EventTypeWarning, podEventDeleteFailed, err.Error())
		return err
	}
	pc.recorder.Event(pod, corev1.EventTypeNormal, podEventDeleteSuccess, "Delete pod in provider successfully")
	log.G(ctx).Debug("Deleted pod from provider")

	return nil
}

func (pc *PodController) updatePodStatus(ctx context.Context, key string) error {
	ctx, span := trace.StartSpan(ctx, "updatePodStatus")
	defer span.End()

	obj, ok := pc.knownPods.Load(key)
	if !ok {
		// This means there was a race and the pod has been deleted from K8s
		return nil
	}
	kPod := obj.(*knownPod)
	kPod.Lock()
	podFromProvider := kPod.lastPodStatusReceivedFromProvider.DeepCopy()
	kPod.Unlock()

	// We need to do this because the other parts of the pod can be updated elsewhere. Since we're only updating
	// the pod status, and we should be the sole writers of the pod status, we can blind overwrite it. Therefore
	// we need to copy the pod and set ResourceVersion to 0.
	podFromProvider.ResourceVersion = "0"
	if _, err := pc.client.Pods(podFromProvider.Namespace).UpdateStatus(ctx, podFromProvider, metav1.UpdateOptions{}); err != nil && !errors.IsNotFound(err) {
		span.SetStatus(err)
		return pkgerrors.Wrap(err, "error while updating pod status in kubernetes")
	}

	log.G(ctx).WithFields(log.Fields{
		"new phase":  string(podFromProvider.Status.Phase),
		"new reason": podFromProvider.Status.Reason,
	}).Debug("Updated pod status in kubernetes")

	return nil
}

// enqueuePodStatusUpdate updates our pod status map, and marks the pod as dirty in the workqueue. The pod must be DeepCopy'd
// prior to enqueuePodStatusUpdate.
func (pc *PodController) enqueuePodStatusUpdate(ctx context.Context, pod *corev1.Pod) {
	ctx, cancel := context.WithTimeout(ctx, notificationRetryPeriod*queue.MaxRetries)
	defer cancel()

	ctx, span := trace.StartSpan(ctx, "enqueuePodStatusUpdate")
	defer span.End()
	ctx = span.WithField(ctx, "method", "enqueuePodStatusUpdate")

	// TODO (Sargun): Make this asynchronousish. Right now, if we are not cache synced, and we receive notifications
	// from the provider for pods that do not exist yet in our known pods map, we can get into an awkward situation.
	key, err := cache.MetaNamespaceKeyFunc(pod)
	if err != nil {
		log.G(ctx).WithError(err).Error("Error getting pod meta namespace key")
		span.SetStatus(err)
		return
	}
	ctx = span.WithField(ctx, "key", key)

	var obj interface{}
	err = wait.PollUntilContextCancel(ctx, notificationRetryPeriod, true, func(ctx context.Context) (bool, error) {
		var ok bool
		obj, ok = pc.knownPods.Load(key)
		if ok {
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		if errors.IsNotFound(err) {
			err = fmt.Errorf("Pod %q not found in pod lister: %w", key, err)
			log.G(ctx).WithError(err).Debug("Not enqueuing pod status update")
		} else {
			log.G(ctx).WithError(err).Warn("Not enqueuing pod status update due to error from pod lister")
		}
		span.SetStatus(err)
		return
	}

	kpod := obj.(*knownPod)
	kpod.Lock()
	if cmp.Equal(kpod.lastPodStatusReceivedFromProvider, pod) {
		kpod.lastPodStatusUpdateSkipped = true
		kpod.Unlock()
		return
	}
	kpod.lastPodStatusUpdateSkipped = false
	kpod.lastPodStatusReceivedFromProvider = pod
	kpod.Unlock()
	pc.syncPodStatusFromProvider.Enqueue(ctx, key)
}

func (pc *PodController) syncPodStatusFromProviderHandler(ctx context.Context, key string) (retErr error) {
	ctx, span := trace.StartSpan(ctx, "syncPodStatusFromProviderHandler")
	defer span.End()

	ctx = span.WithField(ctx, "key", key)
	log.G(ctx).Debug("processing pod status update")
	defer func() {
		span.SetStatus(retErr)
		if retErr != nil {
			log.G(ctx).WithError(retErr).Error("Error processing pod status update")
		}
	}()

	return pc.updatePodStatus(ctx, key)
}

func (pc *PodController) deletePodsFromKubernetesHandler(ctx context.Context, key string) (retErr error) {
	ctx, span := trace.StartSpan(ctx, "deletePodsFromKubernetesHandler")
	defer span.End()

	uid, metaKey := getUIDAndMetaNamespaceKey(key)
	namespace, name, err := cache.SplitMetaNamespaceKey(metaKey)
	ctx = span.WithFields(ctx, log.Fields{
		"namespace": namespace,
		"name":      name,
	})

	if err != nil {
		// Log the error as a warning, but do not requeue the key as it is invalid.
		log.G(ctx).Warn(pkgerrors.Wrapf(err, "invalid resource key: %q", key))
		span.SetStatus(err)
		return nil
	}

	// If the pod has been deleted from API server, we don't need to do anything.
	k8sPod, err := pc.lister.Pods(namespace).Get(name)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		span.SetStatus(err)
		return err
	}
	if string(k8sPod.UID) != uid {
		log.G(ctx).WithField("k8sPodUID", k8sPod.UID).WithField("uid", uid).Warn("Not deleting pod because remote pod has different UID")
		return nil
	}
	if running(&k8sPod.Status) {
		log.G(ctx).Error("Force deleting pod in running state")
	}

	// We don't check with the provider before doing this delete. At this point, even if an outstanding pod status update
	// was in progress,
	deleteOptions := metav1.NewDeleteOptions(0)
	deleteOptions.Preconditions = metav1.NewUIDPreconditions(uid)
	err = pc.client.Pods(namespace).Delete(ctx, name, *deleteOptions)
	if errors.IsNotFound(err) {
		log.G(ctx).Warnf("Not deleting pod because %v", err)
		return nil
	}
	if errors.IsConflict(err) {
		log.G(ctx).Warnf("There was a conflict, maybe trying to delete a Pod that has been recreated: %v", err)
		return nil
	}
	if err != nil {
		span.SetStatus(err)
		return err
	}
	return nil
}

func getUIDAndMetaNamespaceKey(key string) (string, string) {
	idx := strings.LastIndex(key, "/")
	uid := key[idx+1:]
	metaKey := key[:idx]
	return uid, metaKey
}
