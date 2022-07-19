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

# This script builds and runs a local kubernetes cluster.
# Usage: `hack/local-up-cluster.sh`.

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
source "${REPO_ROOT}"/hack/util.sh

# variable define
KUBECONFIG_PATH=${KUBECONFIG_PATH:-"${HOME}/.kube"}
KUBECONFIG=${KUBECONFIG:-"${KUBECONFIG_PATH}/config"}
CLUSTER_NAME=${CLUSTER_NAME:-"kueue-cluster"}
CLUSTER_VERSION=${CLUSTER_VERSION:-"kindest/node:v1.23.4"}
LOG_PATH=${LOG_PATH:-"/tmp"}

# get arch name and os name in bootstrap
BS_ARCH=$(go env GOARCH)
BS_OS=$(go env GOOS)

# Step1. Environment pre-check.
# Make sure go exists and the go version is a viable version.
util::cmd_must_exist "go"
util::verify_go_version

# Make sure docker exists
util::cmd_must_exist "docker"

# precheck or install kind
kind_version=v0.13.0
echo -n "Preparing: 'kind' existence check - "
if util::cmd_exist kind; then
  echo "passed"
else
  echo "not pass"
  util::install_tools "sigs.k8s.io/kind" $kind_version
fi

# precheck or install kubectl
echo -n "Preparing: 'kubectl' existence check - "
if util::cmd_exist kubectl; then
  echo "passed"
else
  echo "not pass"
  util::install_kubectl "" "${BS_ARCH}" "${BS_OS}"
fi

# step2. Create a cluster.
util::create_cluster "${CLUSTER_NAME}" "${KUBECONFIG}" "${CLUSTER_VERSION}" "${LOG_PATH}"

#step3. Wait until the cluster ready
echo "Waiting for the cluster to be ready..."
util::check_clusters_ready "${KUBECONFIG}" "${CLUSTER_NAME}"

function print_success() {
  echo -e "$GREETING"
  echo "Cluster is ready."
}

print_success
