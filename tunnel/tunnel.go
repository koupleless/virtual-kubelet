package tunnel

import (
	"context"
	"github.com/koupleless/arkctl/v1/service/ark"
	"github.com/koupleless/virtual-kubelet/model"
)

// OnBaseDiscovered base discover callback, will start a vnode
type OnBaseDiscovered func(string, model.HeartBeatData, Tunnel)

// OnHealthDataArrived base health data callback, will update vnode status to k8s
type OnHealthDataArrived func(string, ark.HealthData)

// OnQueryAllBizDataArrived biz data callback, will update vpod status to k8s
type OnQueryAllBizDataArrived func(string, []ark.ArkBizInfo)

// OnInstallBizResponseArrived biz install result callback, will update biz-vpod status to k8s
type OnInstallBizResponseArrived func(string, ark.InstallBizResponse)

// OnUnInstallBizResponseArrived biz uninstall result callback, will update biz-vpod status to k8s
type OnUnInstallBizResponseArrived func(string, ark.UnInstallBizResponse)

type Tunnel interface {
	// Key is the identity of Tunnel, will set to node label for special usage
	Key() string

	// Register is the init func of Tunnel, please complete resources init and callback register in this func
	Register(context.Context, string, string, OnBaseDiscovered, OnHealthDataArrived, OnQueryAllBizDataArrived, OnInstallBizResponseArrived, OnUnInstallBizResponseArrived) error

	// OnBaseStart is the func call when a vnode start successfully, you can implement it on demand
	OnBaseStart(context.Context, string)

	// OnBaseStop is the func call when a vnode shutdown successfully, you can implement it on demand
	OnBaseStop(context.Context, string)

	// FetchHealthData is the func call for vnode to fetch health data , you need to fetch health data and call OnHealthDataArrived when data arrived
	FetchHealthData(context.Context, string) error

	// QueryAllBizData is the func call for vnode to fetch all biz data , you need to fetch all biz data and call OnQueryAllBizDataArrived when data arrived
	QueryAllBizData(context.Context, string) error

	// InstallBiz is the func call for vnode to install a biz , you need to start to install biz and call OnInstallBizResponseArrived when install complete with a response
	InstallBiz(context.Context, string, *ark.BizModel) error

	// UninstallBiz is the func call for vnode to uninstall a biz , you need to start to uninstall biz and call OnUnInstallBizResponseArrived when uninstall complete with a response
	UninstallBiz(context.Context, string, *ark.BizModel) error
}
