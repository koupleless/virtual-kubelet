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

package model

// VNodePrefix is a constant string used to prefix vnode names.
const (
	VNodePrefix = "vnode"
)

// TrackSceneVPodDeploy is a constant string used to track the deployment of a vPod.
const (
	TrackSceneVPodDeploy = "vpod_deploy"
)

// These constants are used to identify specific events in the system, allowing for better monitoring and logging capabilities.
const (
	TrackEventContainerStart    = "ContainerStart"    // Represents the event of a container starting.
	TrackEventContainerShutdown = "ContainerShutdown" // Represents the event of a container shutting down.
	TrackEventVPodDelete        = "PodDelete"         // Represents the event of a vPod being deleted.
	TrackEventVPodUpdate        = "PodUpdate"         // Represents the event of a vPod being updated.
)

const (
	// LabelKeyOfTraceID is a constant string used as a key for trace ID in Kubernetes objects.
	LabelKeyOfTraceID = "trace.koupleless.io/id"
	// LabelKeyOfComponent is a constant string used as a key for component in Kubernetes objects.
	LabelKeyOfComponent = "virtual-kubelet.koupleless.io/component"
	// LabelKeyOfEnv is a constant string used as a key for environment in Kubernetes objects.
	LabelKeyOfEnv = "virtual-kubelet.koupleless.io/env"
	// LabelKeyOfBaseName is a constant string used as a key for base name.
	LabelKeyOfBaseName = "base.koupleless.io/name"
	// LabelKeyOfBaseVersion is a constant string used as a key for base version.
	LabelKeyOfBaseVersion = "base.koupleless.io/version"
	// LabelKeyOfBaseClusterName is the constant string used as a key for base cluster name.
	LabelKeyOfBaseClusterName = "base.koupleless.io/cluster-name"
	// LabelKeyOfBaseHostName is a constant string used as a key for base host name.
	LabelKeyOfBaseHostName = "base.koupleless.io/host-name"
	// LabelKeyOfBaseContainerName is a constant string used as a key for base container name.
	LabelKeyOfBaseContainerName = "base.koupleless.io/container-name"
)

const (
	// TaintKeyOfVnode is a constant string used as a key for taints related to virtual nodes in Kubernetes objects.
	TaintKeyOfVnode = "schedule.koupleless.io/virtual-node"
	// TaintKeyOfEnv is a constant string used as a key for taints related to node environments in Kubernetes objects.
	TaintKeyOfEnv = "schedule.koupleless.io/node-env"
)

const (
	// ComponentVNode is a constant string used to identify the vnode component in the system.
	ComponentVNode = "vnode"
	// ComponentVNodeLease is a constant string used to identify the vnode lease component in the system.
	ComponentVNodeLease = "vnode-lease"
)

type ErrorCode string

// CodeSuccess, CodeTimeout, CodeContainerStartTimeout, CodeContainerStartFailed, and CodeContainerStopFailed are constant ErrorCode values representing different error scenarios.
const (
	CodeSuccess               ErrorCode = "00000"
	CodeTimeout               ErrorCode = "00001"
	CodeContainerStartTimeout ErrorCode = "00002"
	CodeContainerStartFailed  ErrorCode = "01002"
	CodeContainerStopFailed   ErrorCode = "01003"
)

// NodeState is the node curr status
type NodeState string

// NodeStateActivated and NodeStateDeactivated are constant NodeState values representing the activation or deactivation of a node.
const (
	// NodeStateActivated node activated, will start vnode if not being started
	NodeStateActivated NodeState = "ACTIVATED"

	// NodeStateDeactivated node deactivated, will shut down vnode if started
	NodeStateDeactivated NodeState = "DEACTIVATED"
)

// BizState is the state of a container, will set to pod state and show on k8s
type BizState string

// BizStateActivated, BizStateResolved, BizStateDeactivated, and ContainerStateWaiting are constant BizState values representing different states of a container.
const (
	// BizStateResolved means container starting
	BizStateResolved BizState = "RESOLVED" // Waiting
	// BizStateUnResolved means uninstall succeed
	BizStateUnResolved BizState = "UNRESOLVED" // -> Terminated
	// BizStateActivated means container ready
	BizStateActivated BizState = "ACTIVATED" // -> Running
	// BizStateDeactivated means container down or broken
	BizStateDeactivated BizState = "DEACTIVATED" // -> Waiting
	// BizStateBroken means install or uninstall failed
	BizStateBroken BizState = "BROKEN" // Waiting
	// BizStateStopped means biz stopped
	BizStateStopped BizState = "STOPPED" // Terminated
)

const (
	// NodeLeaseDurationSeconds is the duration of a node lease in seconds.
	NodeLeaseDurationSeconds = 40
	// NodeLeaseUpdatePeriodSeconds is the period of updating a node lease in seconds.
	NodeLeaseUpdatePeriodSeconds = 10
	// NodeLeaseMaxRetryTimes is the maximum number of times to retry updating a node lease.
	NodeLeaseMaxRetryTimes = 5

	// NodeToUnreachableMaxSeconds is the maximum unreachable duration, if latest heart beat + NodeToUnreachableMaxSeconds > time.now, the vnode is unreachable
	NodeToUnreachableMaxSeconds = 25
	// NodeToDeadMaxSeconds is the maximum offline duration,  if latest heart beat + NodeToDeadMaxSeconds > time.now, the vnode is dead
	NodeToDeadMaxSeconds = 60

	// NodeToFetchHeartBeatInterval is the interval to fetch node's heart beat, NodeToFetchHeartBeatInterval should be less than NodeToUnreachableMaxSeconds
	NodeToFetchHeartBeatInterval = 10
	// NodeToFetchAllBizStatusInterval is the interval to fetch biz's status
	NodeToFetchAllBizStatusInterval = 15
	// NodeToCheckUnreachableAndDeadStatusInterval is the interval to check if node status is unreachable or dead
	NodeToCheckUnreachableAndDeadStatusInterval = 3
)

const (
	// StateReasonAwaitingResync is a constant string used to indicate that a resource is awaiting resynchronization.
	StateReasonAwaitingResync = "AwaitingResync"
)
