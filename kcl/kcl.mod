[package]
name = "deploy-clusterscope"
version = "0.1.0"
description = "KCL module for deploying clusterscope on Kubernetes"

[dependencies]
k8s = "1.31"

[profile]
entries = ["main.k"]
