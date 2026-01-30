# To install prerequisits:
#
# To install redoc-cli:
# $ npm install
#
# To install oapi-codegen in $GOPATH/bin, go outside this go module:
# $ go get github.com/deepmap/oapi-codegen/cmd/oapi-codegen

all: generated
generated: oapi-doc oapi-go

OAPI_CODEGEN=go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen

API_REST_SPEC=./openapi/openapi.yaml
API_REST_CODE_GEN_LOCATION=./openapi/generated/oapigen/oapigen.go
API_REST_DOCO_GEN_LOCATION=./openapi/generated/doc.html
IMAGE_NAME?=registry.gitlab.com/thorchain/midgard

# Open API Makefile targets
oapi-validate:
	./node_modules/.bin/oas-validate -v ${API_REST_SPEC}

oapi-go: oapi-validate
	${OAPI_CODEGEN} --package oapigen --generate types,spec -o ${API_REST_CODE_GEN_LOCATION} ${API_REST_SPEC}

oapi-doc: oapi-validate
	./node_modules/.bin/redoc-cli build ${API_REST_SPEC} -o ${API_REST_DOCO_GEN_LOCATION}

test:
	go test -p 1 -v ./...

lint:
	golangci-lint run -v

format:
	gofumpt -w .

build:
	docker pull ${IMAGE_NAME} || true
	docker build --cache-from ${IMAGE_NAME} -t ${IMAGE_NAME} .
