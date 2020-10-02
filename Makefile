KIND_VERSION = v0.8.1
KIND_CLUSTER_NAME = vault-integration-test
GOPATH = $(shell if [ -x "$$(command -v go)" ]; then go env GOPATH | cut -d: -f1; fi)
TESTFLAGS = -mod=vendor -timeout=20m -v -race -cpu=4
PKGS = $(shell if [ -x "$$(command -v go)" ]; then go list ./... | grep -ivE '(pkg/vault)'; fi)
ARGS = -args -v=1 -logtostderr
export KIND_KUBECONFIG = /tmp/kind-kubeconfig

default: kind-test-cluster

$(GOPATH)/bin/kind:
	sudo apt update
	sudo apt install snapd
	sudo snap install --classic --channel=1.14/stable go
	export PATH=/snap/bin:$PATH
	GO111MODULE="on" go get sigs.k8s.io/kind@$(KIND_VERSION)

kind-test-cluster: $(GOPATH)/bin/kind
	@if [ ! -n "$$($(GOPATH)/bin/kind get clusters 2>/dev/null | grep vault-integration-test)" ]; then \
			$(GOPATH)/bin/kind create cluster --config kind.yaml --name $(KIND_CLUSTER_NAME) --wait 1m \
		;fi && \
	$(GOPATH)/bin/kind get kubeconfig --name $(KIND_CLUSTER_NAME) > $(KIND_KUBECONFIG)

test-ci:
	@CGO_ENABLED=1 go test $(TESTFLAGS) -cover -covermode=atomic -coverpkg="$(shell echo "$(PKGS)" | sed 's/ /,/g')" $(PKGS) $(ARGS)

test-vault:
	@CGO_ENABLED=1 go test $(TESTFLAGS) -cover -covermode=atomic ./pkg/vault $(ARGS)

clean:
	@if [ -x "$$(command -v $(GOPATH)/bin/kind)" ]; then $(GOPATH)/bin/kind delete cluster --name $(KIND_CLUSTER_NAME) > /dev/null 2>&1 ; fi