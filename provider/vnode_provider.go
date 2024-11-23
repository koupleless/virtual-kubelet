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

package provider

import (
	"context"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"k8s.io/apimachinery/pkg/types"
	"sync"

	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet"
	corev1 "k8s.io/api/core/v1"
)

// The following line ensures that VNodeProvider implements the NodeProvider interface.
var _ virtual_kubelet.NodeProvider = &VNodeProvider{}

// VNodeProvider is a struct that implements the NodeProvider interface.
type VNodeProvider struct {
	sync.Mutex

	nodeConfig *model.BuildVNodeConfig // Configuration for building a virtual node provider.

	notify func(*corev1.Node) // Function to notify about node status changes.
}

// Notify updates the latest node status data and notifies about the change.
func (v *VNodeProvider) Notify(data model.NodeStatusData) {
	v.Lock()
	defer v.Unlock()
	node := &corev1.Node{}
	ctx := context.Background()
	err := v.nodeConfig.KubeCache.Get(ctx, types.NamespacedName{Name: v.nodeConfig.NodeName}, node)
	if err != nil {
		log.G(ctx).WithError(err).Error("failed to get node when try to notify status update.")
		return
	}
	vnodeCopy := utils.MergeNodeFromProvider(node, data)
	v.notify(vnodeCopy)
}

// NewVNodeProvider creates a new VNodeProvider instance.
func NewVNodeProvider(config *model.BuildVNodeConfig) *VNodeProvider {
	return &VNodeProvider{
		nodeConfig: config,
	}
}

// Ping checks the health of the node provider.
func (v *VNodeProvider) Ping(_ context.Context) error {
	return nil
}

// NotifyNodeStatus sets a callback function to notify about node status changes.
func (v *VNodeProvider) NotifyNodeStatus(_ context.Context, cb func(*corev1.Node)) {
	v.notify = cb
}
