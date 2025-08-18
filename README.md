<div align="center">

English | [简体中文](./README-zh_CN.md)

</div>

# Virtual Kubelet

[![codecov](https://codecov.io/github/koupleless/virtual-kubelet/graph/badge.svg?token=C0fiHG0xho)](https://codecov.io/github/koupleless/virtual-kubelet)

**Virtual Kubelet** is the **multi-tenant Virtual Kubelet** infrastructure of Koupleless. It has been refactored from the open-source Virtual Kubelet to support managing multiple Virtual Kubelets in a single process.

This is achieved through the implementation of the Tunnel interface, which allows custom resources to be disguised as K8S Nodes.

## Code Structure
1. controller: Control plane components
2. tunnel: Operational pipeline support
3. virtual_kubelet: Original Virtual Kubelet logic, including node information maintenance, pod information maintenance, and other logic
4. vnode: Virtual Kubelet provider implementation
