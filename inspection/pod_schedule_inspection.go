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
	"sync"
	"time"
)

var _ Inspection = &PodScheduleInspection{}

// PodScheduleInspection inspect the pod always in pending status
type PodScheduleInspection struct {
	sync.Mutex

	kubeClient kubernetes.Interface
	ownerMap   map[string]int64
}

func (p *PodScheduleInspection) Register(kubeClient kubernetes.Interface) {
	p.kubeClient = kubeClient
	p.ownerMap = make(map[string]int64)
}

func (p *PodScheduleInspection) GetInterval() time.Duration {
	return time.Second * 60
}

func (p *PodScheduleInspection) Inspect(ctx context.Context, env string) {
	requirement, _ := labels.NewRequirement(model.LabelKeyOfScheduleAnythingComponent, selection.In, []string{model.ComponentVPod})
	envRequirement, _ := labels.NewRequirement(model.LabelKeyOfEnv, selection.In, []string{env})

	// get all module pods with pending phase
	modulePods, err := p.kubeClient.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		LabelSelector: labels.NewSelector().Add(*requirement, *envRequirement).String(),
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
			if len(pod.OwnerReferences) != 0 && p.ownerReported(string(pod.OwnerReferences[0].UID), 10*60) {
				continue
			}
			tracker.G().ErrorReport(pod.Labels[model.LabelKeyOfTraceID], model.TrackSceneVPodDeploy, model.TrackEventVPodSchedule, pod.Status.Conditions[0].Message, pod.Labels, model.CodeVPodScheduleFailed)
		}
	}
}

func (p *PodScheduleInspection) ownerReported(uid string, expireSeconds int64) bool {
	p.Lock()
	defer p.Unlock()
	now := time.Now().Unix()
	if p.ownerMap[uid] >= now-expireSeconds {
		return true
	}
	p.ownerMap[uid] = now
	return false
}
