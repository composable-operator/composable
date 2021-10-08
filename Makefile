
# Image URL to use all building/pushing image targets
IMG ?= cloudoperators/composable-controller
# Update CRDs to v1
CRD_OPTIONS ?= "crd:trivialVersions=true, crdVersions=v1"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif


all: manager

# Run tests
test: generate fmt vet manifests
	TEST_USE_EXISTING_CLUSTER=false go test ./api/... ./controllers/... -coverprofile cover.out -test.v -ginkgo.slowSpecThreshold=7

# Run tests with existing cluster
test-existing: generate fmt vet manifests
	TEST_USE_EXISTING_CLUSTER=true go test ./api/... ./controllers/... -coverprofile cover.out -test.v -ginkgo.slowSpecThreshold=7

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./main.go

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	cd config/manager && kustomize edit set image controller=${IMG}
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager-role webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile=./hack/boilerplate.go.txt paths="./..."


# Build the docker image
#docker-build: test
#	docker build . -t ${IMG}

docker-build: check-tag
	docker build --no-cache . -t ${IMG}:${TAG}
	@echo "updating kustomize image patch file for manager resource"
	# sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml


# Push the docker image
docker-push: docker-build
	docker login -u "${DOCKER_USERNAME}" -p ""${DOCKER_PASSWORD}""
	docker push ${IMG}:${TAG}

# make a release for olm and releases
release: check-tag
	python hack/package.py v${TAG}

# make an initial release for olm and releases
release-update: check-tag
	python hack/package.py v${TAG} --is_update


# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.3.0
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

.PHONY: lintall
lintall: fmt lint vet

lint:
# Get golint if it is not installed
ifeq (, $(shell which golint))
	go get golang.org/x/lint/golint
endif
	golint -set_exit_status=true api/ controllers/

# Run the operator-sdk scorecard on latest release
scorecard:
	hack/operator-scorecard.sh 

# Push OLM metadata to private Quay registry
push-olm: check-tag check-quaytoken check-quayns
	operator-courier push olm/v${TAG} ${QUAY_NS} composable-operator ${TAG} "${QUAY_TOKEN}"
	@echo Remember to make https://quay.io/application/${QUAY_NS}/composable public

check-tag:
ifndef TAG
	$(error TAG is undefined! Please set TAG to the latest release tag, using the format x.y.z e.g. export TAG=0.1.1 )
endif

check-quayns:
ifndef QUAY_NS
	$(error QUAY_NS is undefined!) 
endif

check-quaytoken:
ifndef QUAY_TOKEN
	$(error QUAY_TOKEN is undefined!) 
endif
