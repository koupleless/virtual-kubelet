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

const (
	VNodePrefix = "vnode"
)

const (
	TrackSceneVPodDeploy = "module_deploy"
)

const (
	TrackEventContainerStart     = "ContainerStart"
	TrackEventContainerUnInstall = "PodUnInstall"
	TrackEventVPodDelete         = "PodDelete"
	TrackEventVPodUpdate         = "PodUpdate"
)

const (
	LabelKeyOfTraceID      = "trace.koupleless.io/id"
	LabelKeyOfComponent    = "virtual-kubelet.koupleless.io/component"
	LabelKeyOfEnv          = "virtual-kubelet.koupleless.io/env"
	LabelKeyOfVnodeTunnel  = "vnode.koupleless.io/tunnel"
	LabelKeyOfVNodeName    = "vnode.koupleless.io/name"
	LabelKeyOfVNodeVersion = "vnode.koupleless.io/version"
)

const (
	TaintKeyOfVnode = "schedule.koupleless.io/virtual-node"
	TaintKeyOfEnv   = "schedule.koupleless.io/node-env"
)

const (
	ComponentVNode = "vnode"
)

// ErrorCode first two char represent scene of Error, 00 represent Error of module-controller, 01 represent Error of user config
type ErrorCode string

const (
	CodeSuccess                       ErrorCode = "00000"
	CodeTimeout                       ErrorCode = "00001"
	CodeContainerUninstallTimeout     ErrorCode = "00002"
	CodeContainerInstalledButNotReady ErrorCode = "01001"
	CodeContainerInstallFailed        ErrorCode = "01002"
	CodeContainerStopFailed           ErrorCode = "01003"
)
