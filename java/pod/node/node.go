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

package node

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/pkg/errors"
	"github.com/virtual-kubelet/virtual-kubelet/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/koupleless/virtual-kubelet/common/helper"
	"github.com/koupleless/virtual-kubelet/java/common"
	"github.com/koupleless/virtual-kubelet/java/model"
	"github.com/virtual-kubelet/virtual-kubelet/node"
	corev1 "k8s.io/api/core/v1"
)

type NodeProvider interface {
	node.NodeProvider

	// Register configure node on first attempt
	Register(ctx context.Context, node *corev1.Node) error
}

var _ NodeProvider = &VirtualKubeletNode{}
var modelUtils = common.ModelUtils{}

type VirtualKubeletNode struct {
	sync.Mutex
	nodeConfig *model.BuildVirtualNodeConfig
	arkService ark.Service

	port int

	nodeInfo *corev1.Node

	notify func(*corev1.Node)
}

func NewVirtualKubeletNode(arkService ark.Service, arkServicePort string) *VirtualKubeletNode {
	techStack := os.Getenv("TECH_STACK")
	vNodeCapacityStr := os.Getenv("VNODE_POD_CAPACITY")
	if len(vNodeCapacityStr) == 0 {
		vNodeCapacityStr = "1"
	}

	vnode := model.BuildVirtualNodeConfig{
		NodeIP:       os.Getenv("BASE_POD_IP"),
		TechStack:    techStack,
		Version:      os.Getenv("VNODE_VERSION"),
		VPodCapacity: int(helper.MustReturnFirst[int64](strconv.ParseInt(vNodeCapacityStr, 10, 64))),
	}

	return &VirtualKubeletNode{
		nodeConfig: &vnode,
		arkService: arkService,
		port:       helper.MustReturnFirst[int](strconv.Atoi(arkServicePort)),
		notify: func(node *corev1.Node) {
			// default notify func
			log.G(context.Background()).Info("node status callback not registered")
		},
	}
}

func (v *VirtualKubeletNode) Register(_ context.Context, node *corev1.Node) error {
	modelUtils.BuildVirtualNode(v.nodeConfig, v.arkService, node)
	v.Lock()
	v.nodeInfo = node.DeepCopy()
	v.Unlock()
	return nil
}

func (v *VirtualKubeletNode) Ping(ctx context.Context) error {
	// TODO implement base instance healthy check, waiting for arklet to support base liveness check, default 10 second
	_, err := v.arkService.QueryAllBiz(ctx, ark.QueryAllArkBizRequest{
		HostName: model.LoopBackIp,
		Port:     v.port,
	})
	if err != nil {
		return errors.Wrap(err, "base not activated")
	}
	return nil
}

func (v *VirtualKubeletNode) NotifyNodeStatus(_ context.Context, cb func(*corev1.Node)) {
	// todo: sync base status to k8s, call the callback func to submit the node status
	// can only update node status, Annotations and labels, implement it if need to update these information
	v.notify = cb
	// start a timed task, sync node status periodically
	go common.TimedTaskWithInterval("sync node status", time.Second*3, v.checkCapacityAndNotify)
}

// check curr base process capacity
func (v *VirtualKubeletNode) checkCapacityAndNotify(ctx context.Context) {
	// TODO do capacity check here, update local node status
	var err error
	v.Lock()
	// node status
	nodeReadyStatus := corev1.ConditionTrue
	nodeReadyMessage := ""
	v.nodeInfo.Status.Phase = corev1.NodeRunning
	if err != nil {
		v.nodeInfo.Status.Phase = corev1.NodePending
		nodeReadyStatus = corev1.ConditionFalse
		nodeReadyMessage = err.Error()
	}
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
	// TODO check curr mem, set mem pressure
	v.nodeInfo.Status.Conditions = conditions
	// TODO calculate mem based on arklet
	v.Unlock()
	v.notify(v.nodeInfo.DeepCopy())
}
