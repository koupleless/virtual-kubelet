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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
	"time"

	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/trace"
	pkgerrors "github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/clock"
)

const (
	// Annotation with the JSON-serialized last applied node conditions. Based on kube ctl apply. Used to calculate
	// the three-way patch
	virtualKubeletLastNodeAppliedNodeStatus = "virtual-kubelet.io/last-applied-node-status"
	virtualKubeletLastNodeAppliedObjectMeta = "virtual-kubelet.io/last-applied-object-meta"
)

var (
	// ErrConflictingLeaseControllerConfiguration is returned when the lease controller related options have been
	// specified multiple times
	ErrConflictingLeaseControllerConfiguration = pkgerrors.New("Multiple, conflicting lease configurations have been put into place")
)

// NodeProvider is the interface used for registering a node and updating its
// status in Kubernetes.
//
// Note: Implementers can choose to manage a node themselves, in which case
// it is not needed to provide an implementation for this interface.
type NodeProvider interface { //nolint:revive
	// Ping checks if the node is still active.
	// This is intended to be lightweight as it will be called periodically as a
	// heartbeat to keep the node marked as ready in Kubernetes.
	Ping(context.Context) error

	// NotifyNodeStatus is used to asynchronously monitor the node.
	// The passed in callback should be called any time there is a change to the
	// node's status.
	// This will generally trigger a call to the Kubernetes API server to update
	// the status.
	//
	// NotifyNodeStatus should not block callers.
	NotifyNodeStatus(ctx context.Context, cb func(*corev1.Node))
}

// NewNodeController creates a new node controller.
// This does not have any side-effects on the system or kubernetes.
//
// Use the node's `Run` method to register and run the loops to update the node
// in Kubernetes.
//
// Note: When if there are multiple NodeControllerOpts which apply against the same
// underlying options, the last NodeControllerOpt will win.
func NewNodeController(p NodeProvider, node *corev1.Node, client client.Client, opts ...NodeControllerOpt) (*NodeController, error) {
	n := &NodeController{
		p:          p,
		serverNode: node,
		client:     client,
		chReady:    make(chan struct{}),
		chDone:     make(chan struct{}),
	}
	for _, o := range opts {
		if err := o(n); err != nil {
			return nil, pkgerrors.Wrap(err, "error applying node option")
		}
	}

	if n.pingInterval == time.Duration(0) {
		n.pingInterval = DefaultPingInterval
	}
	if n.statusInterval == time.Duration(0) {
		n.statusInterval = DefaultStatusUpdateInterval
	}

	n.nodePingController = newNodePingController(n.p, n.pingInterval, n.pingTimeout)

	return n, nil
}

// NodeControllerOpt are the functional options used for configuring a node
type NodeControllerOpt func(*NodeController) error //nolint:revive

// WithNodeEnableLeaseV1 enables support for v1 leases.
// V1 Leases share all the same properties as v1beta1 leases, except they do not fallback like
// the v1beta1 lease controller does if the API server does not support it. If the lease duration is not specified (0)
// then DefaultLeaseDuration will be used
func WithNodeEnableLeaseV1(client client.Client, leaseDurationSeconds int32) NodeControllerOpt {
	if leaseDurationSeconds == 0 {
		leaseDurationSeconds = DefaultLeaseDuration
	}

	interval := float64(leaseDurationSeconds) * DefaultRenewIntervalFraction
	intervalDuration := time.Second * time.Duration(int(interval))

	return WithNodeEnableLeaseV1WithRenewInterval(client, leaseDurationSeconds, intervalDuration)
}

// WithNodeEnableLeaseV1WithRenewInterval enables support for v1 leases, and sets a specific renew interval,
// as opposed to the standard multiplier specified by DefaultRenewIntervalFraction
func WithNodeEnableLeaseV1WithRenewInterval(client client.Client, leaseDurationSeconds int32, interval time.Duration) NodeControllerOpt {
	if client == nil {
		panic("client is nil")
	}

	if leaseDurationSeconds == 0 {
		leaseDurationSeconds = DefaultLeaseDuration
	}

	return func(n *NodeController) error {
		if n.leaseController != nil {
			return ErrConflictingLeaseControllerConfiguration
		}

		leaseController, err := newLeaseControllerWithRenewInterval(
			&clock.RealClock{},
			client,
			leaseDurationSeconds,
			interval,
			n,
		)
		if err != nil {
			return fmt.Errorf("Unable to configure lease controller: %w", err)
		}

		n.leaseController = leaseController
		return nil
	}
}

// WithNodeStatusUpdateErrorHandler adds an error handler for cases where there is an error
// when updating the node status.
// This allows the caller to have some control on how errors are dealt with when
// updating a node's status.
//
// The error passed to the handler will be the error received from kubernetes
// when updating node status.
func WithNodeStatusUpdateErrorHandler(h ErrorHandler) NodeControllerOpt {
	return func(n *NodeController) error {
		n.nodeStatusUpdateErrorHandler = h
		return nil
	}
}

// ErrorHandler is a type of function used to allow callbacks for handling errors.
// It is expected that if a nil error is returned that the error is handled and
// progress can continue (or a retry is possible).
type ErrorHandler func(context.Context, error) error

// NodeController deals with creating and managing a node object in Kubernetes.
// It can register a node with Kubernetes and periodically update its status.
// NodeController manages a single node entity.
type NodeController struct { //nolint:revive
	p NodeProvider

	// serverNode must be updated each time it is updated in API Server
	serverNodeLock sync.Mutex
	serverNode     *corev1.Node
	client         client.Client

	leaseController *leaseController

	pingInterval   time.Duration
	statusInterval time.Duration
	chStatusUpdate chan *corev1.Node

	nodeStatusUpdateErrorHandler ErrorHandler

	// chReady is closed once the controller is ready to start the control loop
	chReady chan struct{}
	// chDone is closed once the control loop has exited
	chDone chan struct{}
	errMu  sync.Mutex
	err    error

	nodePingController *nodePingController
	pingTimeout        *time.Duration

	group wait.Group
}

// The default intervals used for lease and status updates.
const (
	DefaultPingInterval         = 10 * time.Second
	DefaultStatusUpdateInterval = 1 * time.Minute
)

// Run registers the node in kubernetes and starts loops for updating the node
// status in Kubernetes.
//
// The node status must be updated periodically in Kubernetes to keep the node
// active. Newer versions of Kubernetes support node leases, which are
// essentially light weight pings. Older versions of Kubernetes require updating
// the node status periodically.
//
// If Kubernetes supports node leases this will use leases with a much slower
// node status update (because some things still expect the node to be updated
// periodically), otherwise it will only use node status update with the configured
// ping interval.
func (n *NodeController) Run(ctx context.Context) (retErr error) {
	defer func() {
		n.errMu.Lock()
		n.err = retErr
		n.errMu.Unlock()
		close(n.chDone)
	}()

	n.chStatusUpdate = make(chan *corev1.Node, 1)
	n.p.NotifyNodeStatus(ctx, func(node *corev1.Node) {
		n.chStatusUpdate <- node
	})

	n.group.StartWithContext(ctx, n.nodePingController.Run)

	n.serverNodeLock.Lock()
	providerNode := n.serverNode.DeepCopy()
	n.serverNodeLock.Unlock()

	if err := n.ensureNode(ctx, providerNode); err != nil {
		return err
	}

	if n.leaseController != nil {
		log.G(ctx).WithField("leaseController", n.leaseController).Debug("Starting leasecontroller")
		n.group.StartWithContext(ctx, n.leaseController.Run)
	}

	return n.controlLoop(ctx, providerNode)
}

// Done signals to the caller when the controller is done and the control loop is exited.
//
// Call n.Err() to find out if there was an error.
func (n *NodeController) Done() <-chan struct{} {
	return n.chDone
}

// Err returns any errors that have occurred that trigger the control loop to exit.
//
// Err only returns a non-nil error after `<-n.Done()` returns.
func (n *NodeController) Err() error {
	n.errMu.Lock()
	defer n.errMu.Unlock()
	return n.err
}

func (n *NodeController) ensureNode(ctx context.Context, providerNode *corev1.Node) (err error) {
	ctx, span := trace.StartSpan(ctx, "node.ensureNode")
	defer span.End()
	defer func() {
		span.SetStatus(err)
	}()

	err = n.updateStatus(ctx, providerNode, true)
	if err == nil || !errors.IsNotFound(err) {
		return err
	}

	n.serverNodeLock.Lock()
	serverNode := n.serverNode
	n.serverNodeLock.Unlock()
	node := serverNode.DeepCopy()
	err = n.client.Create(ctx, node, &client.CreateOptions{})
	if err != nil {
		return pkgerrors.Wrap(err, "error registering node with kubernetes")
	}

	n.serverNodeLock.Lock()
	n.serverNode = node
	n.serverNodeLock.Unlock()
	// Bad things will happen if the node is deleted in k8s and recreated by someone else
	// we rely on this persisting
	providerNode.ObjectMeta.Name = node.Name
	providerNode.ObjectMeta.Namespace = node.Namespace
	providerNode.ObjectMeta.UID = node.UID

	return nil
}

// Ready returns a channel that gets closed when the node is fully up and
// running. Note that if there is an error on startup this channel will never
// be closed.
func (n *NodeController) Ready() <-chan struct{} {
	return n.chReady
}

func (n *NodeController) controlLoop(ctx context.Context, providerNode *corev1.Node) error {
	defer n.group.Wait()

	var sleepInterval time.Duration
	if n.leaseController == nil {
		log.G(ctx).WithField("pingInterval", n.pingInterval).Debug("lease controller is not enabled, updating node status in Kube API server at Ping Time Interval")
		sleepInterval = n.pingInterval
	} else {
		log.G(ctx).WithField("statusInterval", n.statusInterval).Debug("lease controller in use, updating at statusInterval")
		sleepInterval = n.statusInterval
	}

	loop := func() bool {
		ctx, span := trace.StartSpan(ctx, "node.controlLoop.loop")
		defer span.End()

		var timer *time.Timer
		ctx = span.WithField(ctx, "sleepTime", n.pingInterval)
		timer = time.NewTimer(sleepInterval)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return true
		case updated := <-n.chStatusUpdate:
			log.G(ctx).Debug("Received node status update")

			providerNode.Spec.Taints = updated.Spec.Taints
			providerNode.Status = updated.Status
			providerNode.ObjectMeta.Annotations = updated.Annotations
			providerNode.ObjectMeta.Labels = updated.Labels
			if err := n.updateStatus(ctx, providerNode, false); err != nil {
				log.G(ctx).WithError(err).Error("Error handling node status update")
			}
		case <-timer.C:
			if err := n.updateStatus(ctx, providerNode, false); err != nil {
				log.G(ctx).WithError(err).Error("Error handling node status update")
			}
		}
		return false
	}

	close(n.chReady)
	for {
		shouldTerminate := loop()
		if shouldTerminate {
			return nil
		}
	}
}

func (n *NodeController) updateStatus(ctx context.Context, providerNode *corev1.Node, skipErrorCb bool) (err error) {
	ctx, span := trace.StartSpan(ctx, "node.updateStatus")
	defer span.End()
	defer func() {
		span.SetStatus(err)
	}()

	if result, err := n.nodePingController.getResult(ctx); err != nil {
		return err
	} else if result.error != nil {
		return fmt.Errorf("Not updating node status because node ping failed: %w", result.error)
	}

	updateNodeStatusHeartbeat(providerNode)

	node, err := updateNodeStatus(ctx, n.client, providerNode)
	if err != nil {
		if skipErrorCb || n.nodeStatusUpdateErrorHandler == nil {
			return err
		}
		if err := n.nodeStatusUpdateErrorHandler(ctx, err); err != nil {
			return err
		}

		// This might have recreated the node, which may cause problems with our leases until a node update succeeds
		node, err = updateNodeStatus(ctx, n.client, providerNode)
		if err != nil {
			return err
		}
	}

	n.serverNodeLock.Lock()
	n.serverNode = node
	n.serverNodeLock.Unlock()
	return nil
}

// Returns a copy of the server node object
func (n *NodeController) getServerNode(_ context.Context) (*corev1.Node, error) {
	n.serverNodeLock.Lock()
	defer n.serverNodeLock.Unlock()
	if n.serverNode == nil {
		return nil, pkgerrors.New("Server node does not yet exist")
	}
	return n.serverNode.DeepCopy(), nil
}

// just so we don't have to allocate this on every get request
var emptyGetOptions = &client.GetOptions{}

// updateNodeStatus triggers an update to the node status in Kubernetes.
// It first fetches the current node details and then sets the status according
// to the passed in node object.
//
// If you use this function, it is up to you to synchronize this with other operations.
// This reduces the time to second-level precision.
func updateNodeStatus(ctx context.Context, c client.Client, nodeFromProvider *corev1.Node) (_ *corev1.Node, retErr error) {
	ctx, span := trace.StartSpan(ctx, "UpdateNodeStatus")
	defer func() {
		span.End()
		span.SetStatus(retErr)
	}()

	var updatedNode *corev1.Node
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		apiServerNode := &corev1.Node{}
		err := c.Get(ctx, types.NamespacedName{
			Namespace: nodeFromProvider.Namespace,
			Name:      nodeFromProvider.Name,
		}, apiServerNode, emptyGetOptions)
		if err != nil {
			return err
		}
		ctx = addNodeAttributes(ctx, span, apiServerNode)
		log.G(ctx).Debug("got node from api server")
		patch := client.StrategicMergeFrom(apiServerNode)
		updatedNode = nodeFromProvider.DeepCopy()
		err = c.Status().Patch(ctx, updatedNode, patch, &client.SubResourcePatchOptions{})
		if err != nil {
			// We cannot wrap this error because the kubernetes error module doesn't understand wrapping
			log.G(ctx).WithError(err).Warn("Failed to patch node status")
			return err
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	log.G(ctx).WithField("node.resourceVersion", updatedNode.ResourceVersion).
		WithField("node.Status.Conditions", updatedNode.Status.Conditions).
		Debug("updated node status in api server")
	return updatedNode, nil
}

func updateNodeStatusHeartbeat(n *corev1.Node) {
	now := metav1.NewTime(time.Now())
	for i := range n.Status.Conditions {
		n.Status.Conditions[i].LastHeartbeatTime = now
	}
}

type taintsStringer []corev1.Taint

func (t taintsStringer) String() string {
	var s string
	for _, taint := range t {
		if s == "" {
			s = taint.Key + "=" + taint.Value + ":" + string(taint.Effect)
		} else {
			s += ", " + taint.Key + "=" + taint.Value + ":" + string(taint.Effect)
		}
	}
	return s
}

func addNodeAttributes(ctx context.Context, span trace.Span, n *corev1.Node) context.Context {
	return span.WithFields(ctx, log.Fields{
		"node.UID":    string(n.UID),
		"node.name":   n.Name,
		"node.taints": taintsStringer(n.Spec.Taints).String(),
	})
}
