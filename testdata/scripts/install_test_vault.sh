#!/usr/bin/env bash

set -o errexit -o nounset -o pipefail

# Get Helm
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3
chmod 700 get_helm.sh
./get_helm.sh

# Setup Vault for Testing
helm repo add hashicorp https://helm.releases.hashicorp.com
helm install vault hashicorp/vault --version 0.6.0
sleep 60
/tmp/kubectl config set-context --current --namespace=default
/tmp/kubectl get pods
/tmp/kubectl get svc
/tmp/kubectl port-forward vault-0 8200:8200 > /dev/null 2>&1 &
export VAULT_ADDR=http://127.0.0.1:8200
/tmp/kubectl exec -it vault-0 -- vault operator init -key-shares=1  -key-threshold=1 -format "json"| cat >> init.json
ROOT_TOKEN=$(jq -r '.root_token'  init.json)
UNSEAL_KEY=$(jq -r '.unseal_keys_b64[0]' init.json)
sleep 10
/tmp/kubectl exec -it vault-0 -- vault operator unseal $UNSEAL_KEY
sleep 5
/tmp/kubectl exec -it vault-0 -- vault login $ROOT_TOKEN
sleep 5
/tmp/kubectl exec -it vault-0 -- vault secrets enable -path=foo/ kv
/tmp/kubectl exec -it vault-0 -- vault write foo/test-secret value=new-test
sleep 5
/tmp/kubectl exec -it vault-0 -- vault read foo/test-secret
export VAULT_TOKEN=$ROOT_TOKEN

{
  echo '#!/bin/bash';
  echo 'export VAULT_TOKEN="'$ROOT_TOKEN'"';
  echo 'export VAULT_ADDR=http://127.0.0.1:8200';
} >> ./vault_creds.sh
