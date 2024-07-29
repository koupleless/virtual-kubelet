<div align="center">

English | [简体中文](./README-zh_CN.md)

</div>

ModuleController v2 constitutes the operations and orchestration system of Koupleless, designed to be deployed within your Kubernetes cluster, thereby empowering modular operation capabilities.

## Code Structure
1. **cmd/main.go**: Acts as the primary entry point for the application.
2. **controller**: Houses the control plane components, currently including `base_register_controller`, with future Peer Deployment Controllers for Modules also set to reside in this directory.
    1. **base_register_controller**: Manages the lifecycle of the base and shared resources among multi-tenant Virtual Kubelets. It listens for lifecycle events of module Pods, facilitating logic for module installation, updates, and uninstallation.
3. **samples**: Contains example YAML files illustrating module deployment methodologies, RBAC configurations, and deployment strategies for the module controller.
4. **tunnel**: Facilitates the operations pipeline for the base, currently supporting MQTT-based operations. Future support for HTTP operations pipelines is planned, and users can develop custom operation pipelines according to their business needs by implementing the tunnel interface, which can then be injected during the initialization of `base_register_controller`.
5. **virtual_kubelet**: Incorporates the original Virtual Kubelet logic, encompassing node information maintenance and pod management.
6. **vnode**: Implements virtual nodes, allowing for customization through the implementation of PodProvider and NodeProvider interfaces. Currently, it includes the handling logic for base nodes.

## Reference Documents
To consult the usage manual for ModuleController V2, please refer to [this link](https://koupleless.io/docs/tutorials/module-operation-v2/module-online-and-offline/).
For collaborative documentation and an understanding of the implementation principles behind ModuleController V2, visit [here](https://koupleless.io/docs/contribution-guidelines/module-controller-v2/architecture/).