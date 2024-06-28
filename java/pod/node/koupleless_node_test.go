package node

import (
	"context"
	"fmt"
	"github.com/koupleless/arkctl/v1/service/ark"
	podlet "github.com/koupleless/virtual-kubelet/java/pod/let"
	"github.com/virtual-kubelet/virtual-kubelet/node"
	"github.com/virtual-kubelet/virtual-kubelet/node/nodeutil"
	"gotest.tools/assert"
	"net/http"
	"os"
	"path"
	"runtime"
	"sync"
	"testing"
	"time"
)

const (
	VNodeName = "test-node"
)

var knode *KouplelessNode

type apiServerConfig struct {
	CertPath              string
	KeyPath               string
	CACertPath            string
	Addr                  string
	MetricsAddr           string
	StreamIdleTimeout     time.Duration
	StreamCreationTimeout time.Duration
}

func getAPIConfig() (*apiServerConfig, error) {
	config := apiServerConfig{
		CertPath:   os.Getenv("APISERVER_CERT_LOCATION"),
		KeyPath:    os.Getenv("APISERVER_KEY_LOCATION"),
		CACertPath: os.Getenv("APISERVER_CA_CERT_LOCATION"),
	}

	config.Addr = fmt.Sprintf(":%d", 10250)
	config.MetricsAddr = ":10255"
	config.StreamIdleTimeout = 30 * time.Second
	config.StreamCreationTimeout = 30 * time.Second

	return &config, nil
}

func TestNewKouplelessNode(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "env", "test")
	// Ensure API client.
	homeDir, err := os.UserHomeDir()
	assert.NilError(t, err)

	kubePath := path.Join(homeDir, ".kube", "config")
	clientSet, err := nodeutil.ClientsetFromEnv(kubePath)
	assert.NilError(t, err)
	os.Setenv("BASE_POD_KUBE_CONFIG_PATH", kubePath)

	mux := http.NewServeMux()
	apiConfig, err := getAPIConfig()
	assert.NilError(t, err)

	var provider *podlet.BaseProvider
	cm, err := nodeutil.NewNode(
		VNodeName,
		func(config nodeutil.ProviderConfig) (nodeutil.Provider, node.NodeProvider, error) {
			service := ark.BuildService(context.Background())
			nodeProvider := NewVirtualKubeletNode(service)
			// initialize node spec on bootstrap
			nodeProvider.Register(ctx, config.Node)
			provider = podlet.NewBaseProvider(config.Node.Namespace, service, clientSet)
			return provider, nodeProvider, nil
		},
		func(cfg *nodeutil.NodeConfig) error {
			cfg.KubeconfigPath = kubePath
			cfg.Handler = mux
			cfg.InformerResyncPeriod = time.Minute
			cfg.NodeSpec.Status.NodeInfo.Architecture = runtime.GOARCH
			cfg.NodeSpec.Status.NodeInfo.OperatingSystem = runtime.GOOS
			cfg.HTTPListenAddr = apiConfig.Addr
			cfg.StreamCreationTimeout = apiConfig.StreamCreationTimeout
			cfg.StreamIdleTimeout = apiConfig.StreamIdleTimeout
			cfg.DebugHTTP = true

			cfg.NumWorkers = 1
			return nil
		},
		nodeutil.WithClient(clientSet),
		nodeutil.AttachProviderRoutes(mux),
	)
	assert.NilError(t, err)
	os.Setenv("BASE_POD_NAME", "test-deployment-6cb6fd95cd-lxhn9")
	os.Setenv("BASE_POD_NAMESPACE", "default")
	knode, err = NewKouplelessNode(cm, clientSet, VNodeName)
	assert.NilError(t, err)
	assert.Assert(t, knode != nil)
	assert.Assert(t, knode.vkNode != nil)
	assert.Assert(t, knode.bpc != nil)
}

func TestKouplelessNode_Run(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, "env", "test")
	wg := sync.WaitGroup{}
	wg.Add(1)
	go knode.Run(ctx, 1)
	err := knode.WaitReady(ctx, time.Minute)
	assert.NilError(t, err)
}
