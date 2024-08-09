package module_deployment_controller

import (
	"context"
	"errors"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/model"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"time"
)

// Prometheus metric
var ()

func init() {
}

type ModuleDeploymentController struct {
	config *BuildModuleDeploymentControllerConfig

	done  chan struct{}
	ready chan struct{}

	err error

	localStore *RuntimeInfoStore
}

func NewModuleDeploymentController(config *BuildModuleDeploymentControllerConfig) (*ModuleDeploymentController, error) {
	if config == nil {
		return nil, errors.New("config must not be nil")
	}

	return &ModuleDeploymentController{
		config:     config,
		done:       make(chan struct{}),
		ready:      make(chan struct{}),
		localStore: NewRuntimeInfoStore(),
	}, nil
}

func (mdc *ModuleDeploymentController) Run(ctx context.Context) {
	var err error

	ctx, cancel := context.WithCancel(ctx)

	defer func() {
		cancel()
		mdc.err = err
		close(mdc.done)
	}()

	// first sync node cache
	nodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfModuleControllerComponent, selection.In, []string{model.ModuleControllerComponentVNode})
	nodeEnvRequirement, _ := labels.NewRequirement(model.LabelKeyOfEnv, selection.In, []string{mdc.config.Env})

	vnodeInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		mdc.config.K8SConfig.KubeClient,
		mdc.config.K8SConfig.InformerSyncPeriod,
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			// filter all module pods
			options.LabelSelector = labels.NewSelector().Add(*nodeRequirement, *nodeEnvRequirement).String()
		}),
	)

	vnodeInformer := vnodeInformerFactory.Core().V1().Nodes()
	vnodeInformerFactory.Start(ctx.Done())

	// Wait for the caches to be synced *before* starting to do work.
	if ok := cache.WaitForCacheSync(ctx.Done(), vnodeInformer.Informer().HasSynced); !ok {
		err = errors.New("failed to wait for vnode caches to sync")
		return
	}

	var vnodeEventHandler cache.ResourceEventHandler = cache.ResourceEventHandlerFuncs{
		AddFunc:    mdc.vnodeAddHandler,
		UpdateFunc: mdc.vnodeUpdateHandler,
		DeleteFunc: mdc.vnodeDeleteHandler,
	}

	_, err = vnodeInformer.Informer().AddEventHandler(vnodeEventHandler)
	if err != nil {
		return
	}

	// sync deployment cache
	deploymentRequirement, _ := labels.NewRequirement(model.LabelKeyOfModuleControllerComponent, selection.In, []string{model.ModuleControllerComponentModule})

	deploymentInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		mdc.config.K8SConfig.KubeClient,
		mdc.config.K8SConfig.InformerSyncPeriod,
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			// filter all module pods
			options.LabelSelector = labels.NewSelector().Add(*deploymentRequirement).String()
		}),
	)

	deploymentInformer := deploymentInformerFactory.Apps().V1().Deployments()
	deploymentInformerFactory.Start(ctx.Done())

	// Wait for the caches to be synced *before* starting to do work.
	if ok := cache.WaitForCacheSync(ctx.Done(), deploymentInformer.Informer().HasSynced); !ok {
		err = errors.New("failed to wait for deployments caches to sync")
		return
	}

	log.G(ctx).Info("Pod cache in-sync")

	// TODO implement
	var deploymentEventHandler cache.ResourceEventHandler = cache.ResourceEventHandlerFuncs{
		AddFunc:    mdc.deploymentAddHandler,
		UpdateFunc: mdc.deploymentUpdateHandler,
		DeleteFunc: mdc.deploymentDeleteHandler,
	}

	_, err = deploymentInformer.Informer().AddEventHandler(deploymentEventHandler)
	if err != nil {
		return
	}

	close(mdc.ready)
	log.G(ctx).Info("Module deployment controller ready")

	select {
	case <-mdc.done:
	case <-ctx.Done():
	}
}

func (mdc *ModuleDeploymentController) vnodeAddHandler(node interface{}) {
	// TODO
}

func (mdc *ModuleDeploymentController) vnodeUpdateHandler(oldNode, newNode interface{}) {
	// TODO
}

func (mdc *ModuleDeploymentController) vnodeDeleteHandler(node interface{}) {
	// TODO
}

func (mdc *ModuleDeploymentController) deploymentAddHandler(dep interface{}) {
	// TODO
}

func (mdc *ModuleDeploymentController) deploymentUpdateHandler(oldDep, newDep interface{}) {
	// TODO
}

func (mdc *ModuleDeploymentController) deploymentDeleteHandler(dep interface{}) {
	// TODO
}

func (mdc *ModuleDeploymentController) Done() chan struct{} {
	return mdc.done
}

func (mdc *ModuleDeploymentController) Ready() chan struct{} {
	return mdc.ready
}

func (mdc *ModuleDeploymentController) WaitReady(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	select {
	case <-mdc.Ready():
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (mdc *ModuleDeploymentController) Err() error {
	return mdc.err
}
