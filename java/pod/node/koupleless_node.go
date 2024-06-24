package node

import (
	"context"
	base_pod "github.com/koupleless/module-controller/java/pod/base_pod_controller"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	"k8s.io/client-go/kubernetes"
	"os"
	"time"
)

type KouplelessNode struct {
	// vkNode virtual kubelet node, including base node controller and pod controller
	vkNode *nodeutil.Node

	// bpc base pod controller
	bpc *base_pod.BasePodController

	done chan struct{}

	err error
}

func (n *KouplelessNode) Run(ctx context.Context, podSyncWorkers int) (retErr error) {
	// process vkNode run and bpc run, catching error
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()

		n.err = retErr
		close(n.done)
	}()

	if n.bpc != nil {
		go n.bpc.Run(ctx, podSyncWorkers) //nolint:errcheck

		defer func() {
			cancel()
			<-n.bpc.Done()
		}()

		select {
		case <-ctx.Done():
			n.err = ctx.Err()
			return n.err
		case <-n.bpc.Ready():
		case <-n.bpc.Done():
			return n.bpc.Err()
		}

		log.G(ctx).Debug("base pod controller ready")
	}

	go n.runVNode(ctx)

	if n.bpc != nil {
		select {
		case <-ctx.Done():
			cancel()
			n.err = ctx.Err()
			return n.err
		case <-n.Done():
			cancel()
			return n.err
		case <-n.bpc.Done():
			cancel()
			return n.bpc.Err()
		}
	} else {
		select {
		case <-ctx.Done():
			cancel()
			n.err = ctx.Err()
			return n.err
		case <-n.Done():
			cancel()
			return n.err
		}
	}
}

func (n *KouplelessNode) runVNode(ctx context.Context) {
	err := n.vkNode.Run(ctx)
	n.err = err
	close(n.done)
}

// WaitReady waits for the specified timeout for the controller to be ready.
//
// The timeout is for convenience so the caller doesn't have to juggle an extra context.
func (n *KouplelessNode) WaitReady(ctx context.Context, timeout time.Duration) error {
	// complete pod health check
	if n.bpc != nil {
		err := n.bpc.WaitReady(ctx, timeout/2)
		if err != nil {
			return err
		}
	}
	return n.vkNode.WaitReady(ctx, timeout/2)
}

// Done returns a channel that will be closed when the controller has exited.
func (n *KouplelessNode) Done() <-chan struct{} {
	return n.done
}

// NewKouplelessNode creates a new vnode using the provided client and name.
// This is intended for high-level/low boiler-plate usage.
// Use the constructors in the `node` package for lower level configuration.
//
// Some basic values are set for node status, you'll almost certainly want to modify it.
//
// If client is nil, this will construct a client using ClientsetFromEnv
// It is up to the caller to configure auth on the HTTP handler.

func NewKouplelessNode(vnode *nodeutil.Node, vNodeClientSet kubernetes.Interface, vNodeName string) (*KouplelessNode, error) {
	// register vnode and base pod controller
	var bpc *base_pod.BasePodController
	basePodKubeConfigPath := os.Getenv("BASE_POD_KUBE_CONFIG_PATH")
	bpc, err := base_pod.NewBasePodController(base_pod.BasePodControllerConfig{
		VNodeName:             vNodeName,
		VirtualClientSet:      vNodeClientSet,
		BasePodKubeConfigPath: basePodKubeConfigPath,
	})
	if err != nil {
		return nil, errors.Wrap(err, "error creating base pod controller")
	}

	return &KouplelessNode{
		vkNode: vnode,
		bpc:    bpc,
		done:   make(chan struct{}),
	}, nil
}
