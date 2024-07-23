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

package node_provider

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/vnode"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sync"
	"time"
)

type NodeProvider interface {
	vnode.NodeProvider

	// Register configure node on first attempt
	Register(ctx context.Context, node *corev1.Node) error
}

var _ NodeProvider = &JavaBaseNodeProvider{}
var modelUtils = utils.ModelUtils{}

type JavaBaseNodeProvider struct {
	sync.Mutex
	nodeConfig *model.BuildVirtualNodeConfig

	nodeInfo *corev1.Node

	notify func(*corev1.Node)
}

func (v *JavaBaseNodeProvider) Notify(data ark.HealthData) {
	v.Lock()
	defer v.Unlock()
	if v.nodeInfo == nil {
		return
	}
	// node status
	nodeReadyStatus := corev1.ConditionTrue
	nodeReadyMessage := ""
	v.nodeInfo.Status.Phase = corev1.NodeRunning
	conditions := []corev1.NodeCondition{
		{
			Type:   corev1.NodeReady,
			Status: nodeReadyStatus,
			LastHeartbeatTime: metav1.Time{
				Time: time.Now(),
			},
			Message: nodeReadyMessage,
		},
	}
	v.nodeInfo.Status.Conditions = conditions
	if data.Jvm.JavaMaxMetaspace != -1 {
		v.nodeInfo.Status.Capacity[corev1.ResourceMemory] = utils.ConvertByteNumToResourceQuantity(data.Jvm.JavaMaxMetaspace)
	}
	if data.Jvm.JavaCommittedMetaspace != -1 && data.Jvm.JavaMaxMetaspace != -1 {
		v.nodeInfo.Status.Allocatable[corev1.ResourceMemory] = utils.ConvertByteNumToResourceQuantity(data.Jvm.JavaMaxMetaspace - data.Jvm.JavaCommittedMetaspace)
	}
	v.notify(v.nodeInfo.DeepCopy())
}

func NewVirtualKubeletNode(config model.BuildVirtualNodeConfig) *JavaBaseNodeProvider {
	return &JavaBaseNodeProvider{
		nodeConfig: &config,
		notify: func(_ *corev1.Node) {
			// default notify func
			log.G(context.Background()).Info("node status callback not registered")
		},
	}
}

func (v *JavaBaseNodeProvider) Register(_ context.Context, node *corev1.Node) error {
	modelUtils.BuildVirtualNode(v.nodeConfig, node)
	v.Lock()
	v.nodeInfo = node.DeepCopy()
	v.Unlock()
	return nil
}

func (v *JavaBaseNodeProvider) Ping(_ context.Context) error {
	return nil
}

func (v *JavaBaseNodeProvider) NotifyNodeStatus(_ context.Context, cb func(*corev1.Node)) {
	v.notify = cb
}
