package base_pod

import (
	"context"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	"gotest.tools/assert"
	"os"
	"path"
	"testing"
	"time"
)

var basePodController *BasePodController

func TestNewBasePodController(t *testing.T) {
	homeDir, _ := os.UserHomeDir()
	os.Setenv("BASE_POD_NAME", "test-deployment-6cb6fd95cd-lxhn9")
	os.Setenv("BASE_POD_NAMESPACE", "default")
	var err error

	kubeConfigPath := path.Join(homeDir, ".kube", "config")
	clientSet, err := nodeutil.ClientsetFromEnv(kubeConfigPath)
	assert.NilError(t, err)
	basePodController, err = NewBasePodController(BasePodControllerConfig{
		BasePodKubeConfigPath: kubeConfigPath,
		VNodeName:             "vk-koupleless",
		VirtualClientSet:      clientSet,
	})
	assert.NilError(t, err)
}

func TestBasePodControllerRun(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "env", "test")
	go basePodController.Run(ctx, 1)

	err := basePodController.WaitReady(ctx, time.Minute)
	assert.NilError(t, err)
}

func TestFormatModuleFinalizerKey(t *testing.T) {
	key := FormatModuleFinalizerKey("test")
	assert.Equal(t, key, "module-finalizer.koupleless.io/test")
}
