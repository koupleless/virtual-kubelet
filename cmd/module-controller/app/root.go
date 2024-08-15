// Copyright © 2017 The virtual-kubelet authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/koupleless/virtual-kubelet/common/log"
	"github.com/koupleless/virtual-kubelet/common/prometheus"
	"github.com/koupleless/virtual-kubelet/common/tracker"
	"github.com/koupleless/virtual-kubelet/common/utils"
	"github.com/koupleless/virtual-kubelet/controller/base_register_controller"
	"github.com/koupleless/virtual-kubelet/controller/module_deployment_controller"
	"github.com/koupleless/virtual-kubelet/inspection"
	"github.com/koupleless/virtual-kubelet/model"
	"github.com/koupleless/virtual-kubelet/tunnel"
	"github.com/koupleless/virtual-kubelet/tunnel/mqtt_tunnel"
	"github.com/koupleless/virtual-kubelet/virtual_kubelet/nodeutil"
	"github.com/spf13/cobra"
	"time"
)

// NewCommand creates a new top-level command.
// This command is used to start the virtual-kubelet daemon
func NewCommand(ctx context.Context, c Opts) *cobra.Command {
	cmd := &cobra.Command{
		Use: "run",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runModuleControllerCommand(ctx, c)
		},
	}

	installFlags(cmd.Flags(), &c)
	return cmd
}

func runModuleControllerCommand(ctx context.Context, c Opts) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	clientID := uuid.New().String()

	ctx = log.WithLogger(ctx, log.G(ctx).WithFields(log.Fields{
		"operatingSystem": c.OperatingSystem,
		"clientID":        clientID,
		"env":             c.Env,
	}))

	clientSet, err := nodeutil.ClientsetFromEnv(c.KubeConfigPath)
	if err != nil {
		return err
	}

	if c.EnableTracker {
		tracker.SetTracker(&tracker.DefaultTracker{})
	}

	if c.EnablePrometheus {
		go func() {
			err = prometheus.StartPrometheusListen(c.PrometheusPort)
			if err != nil {
				log.G(ctx).WithError(err).Fatal("failed to start prometheus server")
			}
		}()
		log.G(ctx).Infof("Prometheus listening on port %d", c.PrometheusPort)
	}

	if c.EnableInspection {
		for _, insp := range inspection.RegisteredInspection {
			insp.Register(clientSet)
			go utils.TimedTaskWithInterval(ctx, insp.GetInterval(), func(ctx context.Context) {
				insp.Inspect(ctx, c.Env)
			})
		}
	}

	tunnels := make([]tunnel.Tunnel, 0)
	if c.EnableMqttTunnel {
		tunnels = append(tunnels, &mqtt_tunnel.MqttTunnel{})
	}

	moduleDeploymentControllerConfig := module_deployment_controller.BuildModuleDeploymentControllerConfig{
		Env: c.Env,
		K8SConfig: &model.K8SConfig{
			KubeClient:         clientSet,
			InformerSyncPeriod: time.Minute,
		},
		Tunnels: tunnels,
	}

	deploymentController, err := module_deployment_controller.NewModuleDeploymentController(&moduleDeploymentControllerConfig)
	if err != nil {
		return err
	}

	if deploymentController == nil {
		return errors.New("deployment controller is nil")
	}

	go deploymentController.Run(ctx)

	// waiting for register controller ready
	if err = deploymentController.WaitReady(ctx, time.Second*30); err != nil {
		return err
	}

	config := base_register_controller.BuildBaseRegisterControllerConfig{
		ClientID: clientID,
		Env:      c.Env,
		K8SConfig: &model.K8SConfig{
			KubeClient:         clientSet,
			InformerSyncPeriod: time.Minute,
		},
		Tunnels: tunnels,
	}

	registerController, err := base_register_controller.NewBaseRegisterController(&config)
	if err != nil {
		return err
	}

	if registerController == nil {
		return errors.New("register controller is nil")
	}

	go registerController.Run(ctx)

	for _, t := range tunnels {
		err = t.Start(ctx, clientID, c.Env)
		if err != nil {
			log.G(ctx).WithError(err).Error("failed to start tunnel", t.Key())
		} else {
			log.G(ctx).Info("Tunnel started: ", t.Key())
		}
	}

	// waiting for register controller ready
	if err = registerController.WaitReady(ctx, time.Second*30); err != nil {
		return err
	}

	log.G(ctx).Info("Module controller running")

	select {
	case <-ctx.Done():
		log.G(ctx).Error("context canceled")
	case <-registerController.Done():
		log.G(ctx).WithError(registerController.Err()).Error("register controller is stopped")
	}

	return registerController.Err()
}
