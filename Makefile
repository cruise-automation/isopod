GOPATH = $(shell if [ -x "$$(command -v go)" ]; then go env GOPATH | cut -d: -f1; fi)
TESTFLAGS = -mod=vendor -timeout=20m -v -race -cpu=4
PKGS = $(shell if [ -x "$$(command -v go)" ]; then go list ./pkg/... | grep -ivE '(pkg/vault)'; fi)
ARGS = -args -v=1 -logtostderr

test-ci:
	@CGO_ENABLED=1 go test $(TESTFLAGS) -cover -covermode=atomic $(PKGS) $(ARGS)

test-vault:
	@CGO_ENABLED=1 go test $(TESTFLAGS) -cover -covermode=atomic ./pkg/vault $(ARGS)
