#!/usr/bin/env bash

# Copyright 2022 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

# This script creates a cluster with kueue control plane deployed.
REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..

"${REPO_ROOT}"/hack/create-cluster.sh


# Build kueue controller image with source codes.
IMAGE_REGISTRY=registry.cn-shanghai.aliyuncs.com/kerthcet-public make image-build

# Load necessary images.
IMAGE_REGISTRY=registry.cn-shanghai.aliyuncs.com/kerthcet-public make image-load
kind load docker-image gcr.io/kubebuilder/kube-rbac-proxy:v0.8.0

# Deploy kueue controller.
make deploy