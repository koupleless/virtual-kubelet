<div align="center">

[English](./README.md) | 简体中文

</div>

# Virtual Kubelet

[![codecov](https://codecov.io/github/koupleless/virtual-kubelet/graph/badge.svg?token=C0fiHG0xho)](https://codecov.io/github/koupleless/virtual-kubelet)

Virtual Kubelet 是 Koupleless 的多租户 Virtual Kubelet 基础设施。对开源 Virtual Kubelet 进行了多租户改造，支持一个进程中管理多个 Virtual Kubelet。

可以通过对 Tunnel 接口进行实现，实现将自定义资源伪装为 K8S Node。

## 代码结构

1. controller：控制面组件
2. tunnel：运维管道支持
3. virtual_kubelet：原Virtual Kubelet逻辑，包含node信息维护，pod信息维护等逻辑
4. vnode：virtual kubelet provider实现
