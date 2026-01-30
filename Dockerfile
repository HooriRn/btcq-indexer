# Build Image
FROM golang:1.24.6-bullseye AS build

# ca-certificates pull in default CAs, without this https fetch from blockstore will fail
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    build-essential \
    gcc \
    libc6-dev \
    ca-certificates=20210119 && \
    rm -rf /var/lib/apt/lists/*

ENV GOBIN=/go/bin
ENV GOPATH=/go
ENV GOOS=linux

WORKDIR /tmp/midgard

# Cache Go dependencies like this:
COPY go.mod go.sum ./
RUN go mod download

COPY cmd cmd
COPY config config
COPY internal internal
COPY openapi openapi

# Compile.
ENV CC=/usr/bin/gcc
ENV CGO_ENABLED=1
RUN go build -v -installsuffix cgo ./cmd/blockstore/dump
RUN go build -v -installsuffix cgo ./cmd/midgard
RUN go build -v -installsuffix cgo ./cmd/trimdb
RUN go build -v -installsuffix cgo ./cmd/statechecks

# Main Image
FROM debian:bullseye-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    curl \
    wget && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir -p openapi/generated
COPY --from=build /etc/ssl/certs /etc/ssl/certs
COPY --from=build /tmp/midgard/openapi/generated/doc.html ./openapi/generated/doc.html
COPY --from=build /tmp/midgard/dump .
COPY --from=build /tmp/midgard/midgard .
COPY --from=build /tmp/midgard/statechecks .
COPY --from=build /tmp/midgard/trimdb .
COPY --from=build /go/pkg/mod/github.com/!cosm!wasm/wasmvm/v2@v2.1.2/internal/api/libwasmvm.*.so /usr/lib
COPY config/config.json .
COPY resources /resources

CMD [ "./midgard", "config.json" ]
