package nodeutil

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet"
	"k8s.io/client-go/kubernetes/scheme"
	"net/http"
	"path"
	"runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"

	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
)

// Node helps manage the startup/shutdown procedure for other controllers.
// It is intended as a convenience to reduce boiler plate code for starting up controllers.
//
// Must be created with constructor `NewNode`.
type Node struct {
	nc *virtual_kubelet.NodeController
	pc *virtual_kubelet.PodController

	ready chan struct{}
	done  chan struct{}
	err   error

	client client.Client

	listenAddr string
	h          http.Handler
	tlsConfig  *tls.Config

	workers int
}

// NodeController returns the configured node controller.
func (n *Node) NodeController() *virtual_kubelet.NodeController {
	return n.nc
}

// PodController returns the configured pod controller.
func (n *Node) PodController() *virtual_kubelet.PodController {
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
	// Set the runtime client to use
	Client client.Client

	// Set the cache to use
	Cache cache.Cache

	// Set the node spec to register with Kubernetes
	NodeSpec v1.Node

	// Specify the event recorder to use
	// If this is not provided, a default one will be used.
	EventRecorder record.EventRecorder

	// Set the number of workers to reconcile pods
	// The default value is derived from the number of cores available.
	NumWorkers int
}

// WithClient return a NodeOpt that sets the client that will be used to create/manage the node.
func WithClient(c client.Client) NodeOpt {
	return func(cfg *NodeConfig) error {
		cfg.Client = c
		return nil
	}
}

// WithCache return a NodeOpt that sets the cache to fetch local resources.
func WithCache(cache cache.Cache) NodeOpt {
	return func(cfg *NodeConfig) error {
		cfg.Cache = cache
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

	nodeControllerOpts := []virtual_kubelet.NodeControllerOpt{}

	nc, err := virtual_kubelet.NewNodeController(
		np,
		&cfg.NodeSpec,
		cfg.Client,
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

	pc, err := virtual_kubelet.NewPodController(virtual_kubelet.PodControllerConfig{
		EventRecorder: cfg.EventRecorder,
		Client:        cfg.Client,
		Cache:         cfg.Cache,
		Provider:      p,
	})
	if err != nil {
		return nil, errors.Wrap(err, "error creating pod controller")
	}

	return &Node{
		nc:      nc,
		pc:      pc,
		ready:   make(chan struct{}),
		done:    make(chan struct{}),
		client:  cfg.Client,
		workers: cfg.NumWorkers,
	}, nil
}
