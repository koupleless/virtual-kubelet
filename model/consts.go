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
	CommandHealth       = "health"
	CommandQueryAllBiz  = "queryAllBiz"
	CommandInstallBiz   = "installBiz"
	CommandUnInstallBiz = "uninstallBiz"
)

const (
	BaseHeartBeatTopic = "koupleless_%s/+/base/heart"
	BaseHealthTopic    = "koupleless_%s/%s/base/health"
	BaseBizTopic       = "koupleless_%s/%s/base/biz"
)

const (
	BaseNodePrefix = "base-node"
)

const (
	TrackSceneModuleDeployment = "module_deployment"
)

const (
	TrackEventModuleInstall   = "ModuleInstall"
	TrackEventModuleUnInstall = "PodUnInstall"
	TrackEventPodDelete       = "PodDelete"
	TrackEventPodUpdate       = "PodUpdate"
	TrackEventPodSchedule     = "PodSchedule"
)

const (
	PodTraceIDLabelKey                   = "trace.koupleless.io/id"
	PodModuleControllerComponentLabelKey = "module-controller.koupleless.io/component"
)

const (
	ModuleControllerComponentModule = "module"
)

// ErrorCode first two char represent scene of Error, 00 represent Error of module-controller, 01 represent Error of user config
type ErrorCode string

const (
	CodeSuccess                       ErrorCode = "00000"
	CodeTimeout                       ErrorCode = "00001"
	CodeModuleUninstallTimeout        ErrorCode = "00002"
	CodeModuleInstalledButDeactivated ErrorCode = "01001"
	CodeModuleInstallFailed           ErrorCode = "01002"
	CodeModuleUnInstallFailed         ErrorCode = "01003"
	CodeModulePodScheduleFailed       ErrorCode = "01004"
)
