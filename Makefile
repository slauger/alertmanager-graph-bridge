BINARY := alertmanager-graph-bridge
IMAGE := ghcr.io/slauger/alertmanager-graph-bridge
CHART := charts/alertmanager-graph-bridge
COVER_THRESHOLD := 80

# PRODUCT_NAME overrides the compiled-in brand (see internal/branding). Leave
# empty to keep the default baked into the source.
PRODUCT_NAME ?=
ifneq ($(strip $(PRODUCT_NAME)),)
GO_LDFLAGS := -ldflags "-X 'github.com/slauger/alertmanager-graph-bridge/internal/branding.ProductName=$(PRODUCT_NAME)'"
endif

.PHONY: build run test cover lint vet vulncheck fmt tidy unicode-lint \
	helm-lint helm-template helm-unittest docker-build ci clean help \
	e2e-infra e2e-build e2e-run e2e-local \
	cluster-e2e-build cluster-e2e cluster-e2e-keep

help:
	@echo "Targets: build run test cover lint vet vulncheck fmt tidy unicode-lint \
helm-lint helm-template helm-unittest docker-build ci clean \
e2e-infra e2e-build e2e-run e2e-local \
cluster-e2e cluster-e2e-keep"

build:
	go build -trimpath $(GO_LDFLAGS) -o bin/$(BINARY) ./cmd/$(BINARY)

run:
	go run ./cmd/$(BINARY) -config config.yaml

test:
	go test ./... -race

cover:
	go test ./... -race -covermode=atomic -coverprofile=cover.out
	./hack/check-coverage.sh $(COVER_THRESHOLD)

lint:
	golangci-lint run

vet:
	go vet ./...

vulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@v1.3.0 ./...

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

unicode-lint:
	./hack/unicode-lint.sh

helm-lint:
	helm lint $(CHART)

helm-template:
	helm template test $(CHART)

helm-unittest:
	helm unittest $(CHART)

docker-build:
	docker build --build-arg PRODUCT_NAME=$(PRODUCT_NAME) -f images/$(BINARY)/Containerfile -t $(IMAGE):dev .

e2e-infra:
	terraform -chdir=terraform init -upgrade
	terraform -chdir=terraform apply

e2e-build:
	docker build -f images/$(BINARY)-e2e/Containerfile -t $(BINARY)-e2e:dev .

e2e-run:
	docker run --rm --env-file e2e.env $(BINARY)-e2e:dev -test.v

e2e-local:
	go test -tags e2e -v ./test/e2e/...

# Full-chain cluster e2e test: kind cluster + Prometheus + Alertmanager + the
# bridge. Needs only Docker on the host; see docs/cluster-e2e-testing.md.
cluster-e2e-build:
	docker build -f images/cluster-e2e/Containerfile -t $(BINARY)-cluster-e2e:dev .

cluster-e2e: cluster-e2e-build
	docker run --rm \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v "$(CURDIR)":/workspace \
		--env-file e2e.env \
		$(BINARY)-cluster-e2e:dev

# Same as cluster-e2e but leaves the kind cluster running for inspection.
cluster-e2e-keep: cluster-e2e-build
	docker run --rm \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v "$(CURDIR)":/workspace \
		--env-file e2e.env \
		$(BINARY)-cluster-e2e:dev --keep

ci: tidy vet lint vulncheck cover unicode-lint helm-lint helm-unittest

clean:
	rm -rf bin site cover.out *.tgz
