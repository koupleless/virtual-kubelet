package inspection

import (
	"context"
	"github.com/koupleless/virtual-kubelet/common/tracker"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/kubernetes"
	"time"
)

var _ Inspection = &PodScheduleInspection{}

// PodScheduleInspection inspect the pod always in pending status
type PodScheduleInspection struct {
	kubeClient kubernetes.Interface
	ownerMap   map[string]int64
}

func (p *PodScheduleInspection) Register(kubeClient kubernetes.Interface) {
	p.kubeClient = kubeClient
	p.ownerMap = make(map[string]int64)
}

func (p *PodScheduleInspection) GetIntervalMilliSec() time.Duration {
	return time.Second * 60
}

func (p *PodScheduleInspection) Inspect(ctx context.Context) {
	requirement, _ := labels.NewRequirement(model.PodModuleControllerComponentLabelKey, selection.In, []string{model.ModuleControllerComponentModule})

	// get all module pods with pending phase
	modulePods, err := p.kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		LabelSelector: labels.NewSelector().Add(*requirement).String(),
		FieldSelector: fields.OneTermEqualSelector("status.phase", string(v1.PodPending)).String(),
	})
	if err != nil {
		logrus.Errorf("failed to list pods: %v", err)
		return
	}

	for _, pod := range modulePods.Items {
		// select pods with scheduled but schedule failed
		if len(pod.Status.Conditions) != 0 && pod.Status.Conditions[0].Type == v1.PodScheduled && pod.Status.Conditions[0].Status == v1.ConditionFalse {
			// check owner has been reported
			if len(pod.OwnerReferences) != 0 {
				now := time.Now().Unix()
				replicaSetUID := string(pod.OwnerReferences[0].UID)
				if p.ownerMap[replicaSetUID] >= now-10*60 {
					delete(p.ownerMap, replicaSetUID)
					continue
				}
				p.ownerMap[replicaSetUID] = now
			}
			if pod.Labels == nil {
				pod.Labels = make(map[string]string)
			}
			tracker.G().ErrorReport(pod.Labels[model.PodTraceIDLabelKey], model.TrackSceneModuleDeployment, model.TrackEventPodSchedule, pod.Status.Conditions[0].Message, pod.Labels, model.CodeModulePodScheduleFailed)
		}
	}
}
