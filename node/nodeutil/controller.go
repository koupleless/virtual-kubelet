package nodeutil

import (
	"context"
	"crypto/tls"
	"fmt"
	lister "k8s.io/client-go/listers/core/v1"
	"net/http"
	"path"
	"runtime"
	"time"

	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/node"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
)

// Node helps manage the startup/shutdown procedure for other controllers.
// It is intended as a convenience to reduce boiler plate code for starting up controllers.
//
// Must be created with constructor `NewNode`.
type Node struct {
	nc *node.NodeController
	pc *node.PodController

	readyCb func(context.Context) error

	ready chan struct{}
	done  chan struct{}
	err   error

	client kubernetes.Interface

	listenAddr string
	h          http.Handler
	tlsConfig  *tls.Config

	workers int

	eb record.EventBroadcaster
}

// NodeController returns the configured node controller.
func (n *Node) NodeController() *node.NodeController {
	return n.nc
}

// PodController returns the configured pod controller.
func (n *Node) PodController() *node.PodController {
	return n.pc
}

// Run starts all the underlying controllers
func (n *Node) Run(ctx context.Context) (retErr error) {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()

		n.err = retErr
		close(n.done)
	}()

	if n.eb != nil {
		n.eb.StartLogging(log.G(ctx).Infof)
		n.eb.StartRecordingToSink(&corev1client.EventSinkImpl{Interface: n.client.CoreV1().Events(v1.NamespaceAll)})
		defer n.eb.Shutdown()
		log.G(ctx).Debug("Started event broadcaster")
	}

	go n.pc.Run(ctx, n.workers) //nolint:errcheck

	defer func() {
		cancel()
		<-n.pc.Done()
	}()

	select {
	case <-ctx.Done():
		return n.err
	case <-n.pc.Ready():
	case <-n.pc.Done():
		return n.pc.Err()
	}

	log.G(ctx).Debug("pod controller ready")

	go n.nc.Run(ctx) //nolint:errcheck

	defer func() {
		cancel()
		<-n.nc.Done()
	}()

	select {
	case <-ctx.Done():
		n.err = ctx.Err()
		return n.err
	case <-n.nc.Ready():
	case <-n.nc.Done():
		return n.nc.Err()
	}

	log.G(ctx).Debug("node controller ready")

	if n.readyCb != nil {
		if err := n.readyCb(ctx); err != nil {
			return err
		}
	}
	close(n.ready)

	select {
	case <-n.nc.Done():
		cancel()
		return n.nc.Err()
	case <-n.pc.Done():
		cancel()
		return n.pc.Err()
	}
}

// WaitReady waits for the specified timeout for the controller to be ready.
//
// The timeout is for convenience so the caller doesn't have to juggle an extra context.
func (n *Node) WaitReady(ctx context.Context, timeout time.Duration) error {
	if timeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	select {
	case <-n.ready:
		return nil
	case <-n.done:
		return fmt.Errorf("controller exited before ready: %w", n.err)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Ready returns a channel that will be closed after the controller is ready.
func (n *Node) Ready() <-chan struct{} {
	return n.ready
}

// Done returns a channel that will be closed when the controller has exited.
func (n *Node) Done() <-chan struct{} {
	return n.done
}

// Err returns any error that occurred with the controller.
//
// This always return nil before `<-Done()`.
func (n *Node) Err() error {
	select {
	case <-n.Done():
		return n.err
	default:
		return nil
	}
}

// NodeOpt is used as functional options when configuring a new node in NewNodeFromClient
type NodeOpt func(c *NodeConfig) error

// NodeConfig is used to hold configuration items for a Node.
// It gets used in conjection with NodeOpt in NewNodeFromClient
type NodeConfig struct {
	// Set the client to use, otherwise a client will be created from ClientsetFromEnv
	Client kubernetes.Interface

	// Set the PodLister to use
	PodLister lister.PodLister

	// Set the node spec to register with Kubernetes
	NodeSpec v1.Node

	// Specify the event recorder to use
	// If this is not provided, a default one will be used.
	EventRecorder record.EventRecorder

	// Set the number of workers to reconcile pods
	// The default value is derived from the number of cores available.
	NumWorkers int

	// Set the error handler for node status update failures
	NodeStatusUpdateErrorHandler node.ErrorHandler
}

// WithClient return a NodeOpt that sets the client that will be used to create/manage the node.
func WithClient(c kubernetes.Interface) NodeOpt {
	return func(cfg *NodeConfig) error {
		cfg.Client = c
		return nil
	}
}

// WithPodLister return a NodeOpt that sets the pod lister that will be used to create/manage the pod.
func WithPodLister(pl lister.PodLister) NodeOpt {
	return func(cfg *NodeConfig) error {
		cfg.PodLister = pl
		return nil
	}
}

// NewNode creates a new node using the provided client and name.
// This is intended for high-level/low boiler-plate usage.
// Use the constructors in the `node` package for lower level configuration.
//
// Some basic values are set for node status, you'll almost certainly want to modify it.
//
// If client is nil, this will construct a client using ClientsetFromEnv
// It is up to the caller to configure auth on the HTTP handler.
func NewNode(name string, newProvider NewProviderFunc, opts ...NodeOpt) (*Node, error) {
	cfg := NodeConfig{
		NumWorkers: runtime.NumCPU(),
		NodeSpec: v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
				Labels: map[string]string{
					"type":                   "virtual-kubelet",
					"kubernetes.io/role":     "agent",
					"kubernetes.io/hostname": name,
				},
			},
			Status: v1.NodeStatus{
				Phase: v1.NodePending,
				Conditions: []v1.NodeCondition{
					{Type: v1.NodeReady},
					{Type: v1.NodeDiskPressure},
					{Type: v1.NodeMemoryPressure},
					{Type: v1.NodePIDPressure},
					{Type: v1.NodeNetworkUnavailable},
				},
			},
		},
	}

	for _, o := range opts {
		if err := o(&cfg); err != nil {
			return nil, err
		}
	}

	if cfg.Client == nil {
		return nil, errors.New("no client provided")
	}

	p, np, err := newProvider(ProviderConfig{
		Node: &cfg.NodeSpec,
	})
	if err != nil {
		return nil, errors.Wrap(err, "error creating provider")
	}

	var readyCb func(context.Context) error
	if np == nil {
		nnp := node.NewNaiveNodeProvider()
		np = nnp

		readyCb = func(ctx context.Context) error {
			setNodeReady(&cfg.NodeSpec)
			err := nnp.UpdateStatus(ctx, &cfg.NodeSpec)
			return errors.Wrap(err, "error marking node as ready")
		}
	}

	nodeControllerOpts := []node.NodeControllerOpt{
		node.WithNodeEnableLeaseV1(NodeLeaseV1Client(cfg.Client), node.DefaultLeaseDuration),
	}

	if cfg.NodeStatusUpdateErrorHandler != nil {
		nodeControllerOpts = append(nodeControllerOpts, node.WithNodeStatusUpdateErrorHandler(cfg.NodeStatusUpdateErrorHandler))
	}

	nc, err := node.NewNodeController(
		np,
		&cfg.NodeSpec,
		cfg.Client.CoreV1().Nodes(),
		nodeControllerOpts...,
	)
	if err != nil {
		return nil, errors.Wrap(err, "error creating node controller")
	}

	var eb record.EventBroadcaster
	if cfg.EventRecorder == nil {
		eb = record.NewBroadcaster()
		cfg.EventRecorder = eb.NewRecorder(scheme.Scheme, v1.EventSource{Component: path.Join(name, "pod-controller")})
	}

	pc, err := node.NewPodController(node.PodControllerConfig{
		PodClient:     cfg.Client.CoreV1(),
		PodLister:     cfg.PodLister,
		EventRecorder: cfg.EventRecorder,
		Provider:      p,
	})
	if err != nil {
		return nil, errors.Wrap(err, "error creating pod controller")
	}

	return &Node{
		nc:      nc,
		pc:      pc,
		readyCb: readyCb,
		ready:   make(chan struct{}),
		done:    make(chan struct{}),
		eb:      eb,
		client:  cfg.Client,
		workers: cfg.NumWorkers,
	}, nil
}

func setNodeReady(n *v1.Node) {
	n.Status.Phase = v1.NodeRunning
	for i, c := range n.Status.Conditions {
		if c.Type != "Ready" {
			continue
		}

		c.Message = "Kubelet is ready"
		c.Reason = "KubeletReady"
		c.Status = v1.ConditionTrue
		c.LastHeartbeatTime = metav1.Now()
		c.LastTransitionTime = metav1.Now()
		n.Status.Conditions[i] = c
		return
	}
}
