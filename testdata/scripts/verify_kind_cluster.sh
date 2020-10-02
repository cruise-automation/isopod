#!/usr/bin/env bash

set -o errexit -o nounset -o pipefail

# Verify KIND cluster available
curl -LO https://storage.googleapis.com/kubernetes-release/release/`curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt`/bin/linux/amd64/kubectl
chmod +x ./kubectl
mv ./kubectl /tmp/kubectl
/tmp/kubectl cluster-info --context kind-vault-integration-test
/tmp/kubectl get nodes