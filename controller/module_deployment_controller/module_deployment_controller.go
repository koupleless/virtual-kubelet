package module_deployment_controller

import (
	"context"
	"errors"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/tracker"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"sort"
	"time"
)

// Prometheus metric
var (
	PeerDeploymentNum    prometheus.Gauge
	NonPeerDeploymentNum prometheus.Gauge
)

func init() {
	PeerDeploymentNum = promauto.NewGauge(prometheus.GaugeOpts{
		Name:      "peer_deployment_num",
		Help:      "Number of peer deployments",
		Namespace: "module_deployment_controller",
	})
	NonPeerDeploymentNum = promauto.NewGauge(prometheus.GaugeOpts{
		Name:      "non_peer_deployment_num",
		Help:      "Number of non peer deployments",
		Namespace: "module_deployment_controller",
	})
}

type ModuleDeploymentController struct {
	config *BuildModuleDeploymentControllerConfig

	done  chan struct{}
	ready chan struct{}

	err error

	localStore *RuntimeInfoStore

	updateToken chan interface{}
}

func NewModuleDeploymentController(config *BuildModuleDeploymentControllerConfig) (*ModuleDeploymentController, error) {
	if config == nil {
		return nil, errors.New("config must not be nil")
	}

	return &ModuleDeploymentController{
		config:      config,
		done:        make(chan struct{}),
		ready:       make(chan struct{}),
		localStore:  NewRuntimeInfoStore(),
		updateToken: make(chan interface{}, 1),
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

	mdc.updateToken <- nil

	for _, t := range mdc.config.Tunnels {
		t.RegisterQuery(mdc.queryContainerBaseline)
	}

	envRequirement, _ := labels.NewRequirement(model.LabelKeyOfEnv, selection.In, []string{mdc.config.Env})

	// first sync node cache
	nodeRequirement, _ := labels.NewRequirement(model.LabelKeyOfScheduleAnythingComponent, selection.In, []string{model.ComponentVNode})
	vnodeSelector := labels.NewSelector().Add(*nodeRequirement, *envRequirement)

	vnodeInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		mdc.config.K8SConfig.KubeClient,
		mdc.config.K8SConfig.InformerSyncPeriod,
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			// filter all module pods
			options.LabelSelector = vnodeSelector.String()
		}),
	)

	vnodeInformer := vnodeInformerFactory.Core().V1().Nodes()
	vnodeLister := vnodeInformer.Lister()
	vnodeInformerFactory.Start(ctx.Done())

	// Wait for the caches to be synced *before* starting to do work.
	if ok := cache.WaitForCacheSync(ctx.Done(), vnodeInformer.Informer().HasSynced); !ok {
		err = errors.New("failed to wait for vnode caches to sync")
		return
	}

	// vnode init
	vnodeList, err := vnodeLister.List(vnodeSelector)
	if err != nil {
		err = errors.New("failed to list vnode")
		return
	}

	for _, vnode := range vnodeList {
		// no deployment, just add
		mdc.localStore.PutNode(vnode.DeepCopy())
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
	deploymentRequirement, _ := labels.NewRequirement(model.LabelKeyOfScheduleAnythingComponent, selection.In, []string{model.ComponentVPodDeployment})
	deploymentSelector := labels.NewSelector().Add(*deploymentRequirement, *envRequirement)

	deploymentInformerFactory := informers.NewSharedInformerFactoryWithOptions(
		mdc.config.K8SConfig.KubeClient,
		mdc.config.K8SConfig.InformerSyncPeriod,
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			// filter all module pods
			options.LabelSelector = deploymentSelector.String()
		}),
	)

	deploymentInformer := deploymentInformerFactory.Apps().V1().Deployments()
	deploymentLister := deploymentInformer.Lister()
	deploymentInformerFactory.Start(ctx.Done())

	// Wait for the caches to be synced *before* starting to do work.
	if ok := cache.WaitForCacheSync(ctx.Done(), deploymentInformer.Informer().HasSynced); !ok {
		err = errors.New("failed to wait for deployments caches to sync")
		return
	}

	// init deployments
	deploymentList, err := deploymentLister.List(deploymentSelector)
	if err != nil {
		err = errors.New("failed to list deployments")
		return
	}

	for _, deployment := range deploymentList {
		mdc.localStore.PutDeployment(deployment.DeepCopy())
	}

	mdc.updateDeploymentReplicas(deploymentList)

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

func (mdc *ModuleDeploymentController) queryContainerBaseline(req model.QueryBaselineRequest) []*corev1.Container {
	labelMap := map[string]string{
		model.LabelKeyOfEnv:          mdc.config.Env,
		model.LabelKeyOfVNodeName:    req.Name,
		model.LabelKeyOfVNodeVersion: req.Version,
	}
	for key, value := range req.CustomLabels {
		labelMap[key] = value
	}
	relatedDeploymentsByNode := mdc.localStore.GetRelatedDeploymentsByNode(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labelMap,
		},
	})
	// get relate containers of related deployments
	sort.Slice(relatedDeploymentsByNode, func(i, j int) bool {
		return relatedDeploymentsByNode[i].CreationTimestamp.UnixMilli() < relatedDeploymentsByNode[j].CreationTimestamp.UnixMilli()
	})
	// record last version of biz model with same name
	containers := make([]*corev1.Container, 0)
	for _, deployment := range relatedDeploymentsByNode {
		for _, container := range deployment.Spec.Template.Spec.Containers {
			containers = append(containers, &container)
		}
	}
	return containers
}

func (mdc *ModuleDeploymentController) vnodeAddHandler(node interface{}) {
	vnode, ok := node.(*corev1.Node)
	if !ok {
		return
	}
	changed := mdc.localStore.PutNode(vnode.DeepCopy())
	if changed {
		relatedDeploymentsByNode := mdc.localStore.GetRelatedDeploymentsByNode(vnode)
		go mdc.updateDeploymentReplicas(relatedDeploymentsByNode)
	}
}

func (mdc *ModuleDeploymentController) vnodeUpdateHandler(_, newNode interface{}) {
	vnode, ok := newNode.(*corev1.Node)
	if !ok {
		return
	}
	changed := mdc.localStore.PutNode(vnode.DeepCopy())
	if changed {
		relatedDeploymentsByNode := mdc.localStore.GetRelatedDeploymentsByNode(vnode)
		go mdc.updateDeploymentReplicas(relatedDeploymentsByNode)
	}
}

func (mdc *ModuleDeploymentController) vnodeDeleteHandler(node interface{}) {
	vnode, ok := node.(*corev1.Node)
	if !ok {
		return
	}
	vnodeCopy := vnode.DeepCopy()
	mdc.localStore.DeleteNode(vnodeCopy)
	relatedDeploymentsByNode := mdc.localStore.GetRelatedDeploymentsByNode(vnodeCopy)
	go mdc.updateDeploymentReplicas(relatedDeploymentsByNode)
}

func (mdc *ModuleDeploymentController) deploymentAddHandler(dep interface{}) {
	moduleDeployment, ok := dep.(*appsv1.Deployment)
	if !ok {
		return
	}

	deploymentCopy := moduleDeployment.DeepCopy()
	mdc.localStore.PutDeployment(deploymentCopy)

	go mdc.updateDeploymentReplicas([]*appsv1.Deployment{deploymentCopy})
}

func (mdc *ModuleDeploymentController) deploymentUpdateHandler(_, newDep interface{}) {
	moduleDeployment, ok := newDep.(*appsv1.Deployment)
	if !ok {
		return
	}
	deploymentCopy := moduleDeployment.DeepCopy()
	mdc.localStore.PutDeployment(deploymentCopy)

	go mdc.updateDeploymentReplicas([]*appsv1.Deployment{deploymentCopy})
}

func (mdc *ModuleDeploymentController) deploymentDeleteHandler(dep interface{}) {
	moduleDeployment, ok := dep.(*appsv1.Deployment)
	if !ok {
		return
	}
	mdc.localStore.DeleteDeployment(moduleDeployment.DeepCopy())
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

func (mdc *ModuleDeploymentController) updateDeploymentReplicas(deployments []*appsv1.Deployment) {
	<-mdc.updateToken
	defer func() {
		mdc.updateToken <- nil
	}()
	for _, deployment := range deployments {
		if deployment.Labels[model.LabelKeyOfVPodDeploymentStrategy] != string(model.VPodDeploymentStrategyPeer) || deployment.Labels[model.LabelKeyOfSkipReplicasControl] != "true" {
			continue
		}
		newReplicas := mdc.localStore.GetMatchedNodeNum(deployment)
		if newReplicas != *deployment.Spec.Replicas {
			err := tracker.G().FuncTrack(deployment.Labels[model.LabelKeyOfTraceID], model.TrackSceneVPodDeploy, model.TrackEventVPodPeerDeploymentReplicaModify, deployment.Labels, func() (error, model.ErrorCode) {
				return mdc.updateDeploymentReplicasOfKubernetes(newReplicas, deployment)
			})
			if err != nil {
				logrus.WithError(err).Errorf("failed to update deployment replicas of %s", deployment.Name)
			}
		}
	}
}

func (mdc *ModuleDeploymentController) updateDeploymentReplicasOfKubernetes(replicas int32, deployment *appsv1.Deployment) (error, model.ErrorCode) {
	// Create a Scale object
	s := &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{
			Name:      deployment.Name,
			Namespace: deployment.Namespace,
		},
		Spec: autoscalingv1.ScaleSpec{
			Replicas: replicas,
		},
	}

	_, err := mdc.config.K8SConfig.KubeClient.AppsV1().Deployments(deployment.Namespace).UpdateScale(context.Background(), deployment.Name, s, metav1.UpdateOptions{})

	return err, model.CodeKubernetesOperationFailed
}
