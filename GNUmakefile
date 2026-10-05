default: build

build:
	go build -v ./...

install: build
	go install -v ./...

fmt:
	gofmt -s -w -e .

vet:
	go vet ./...

generate:
	go generate ./...

test:
	go test -v -timeout=120s ./...

# Acceptance tests run against the real API and need HOSTD_API_KEY,
# HOSTD_TEST_CLUSTER and HOSTD_TEST_OS_IMAGE (see README.md).
testacc:
	TF_ACC=1 go test -v -timeout=30m ./...

.PHONY: default build install fmt vet generate test testacc
