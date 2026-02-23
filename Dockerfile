# Build Image
FROM golang:1.25.7 AS build

# ca-certificates pull in default CAs, without this https fetch from blockstore will fail
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    build-essential \
    gcc \
    libc6-dev \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

ENV GOBIN=/go/bin
ENV GOPATH=/go
ENV GOOS=linux

WORKDIR /tmp/btcq-indexer

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

RUN go build -v -installsuffix cgo ./cmd/btcq-indexer


# Main Image
FROM debian:bullseye-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    curl \
    wget && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir -p openapi/generated
COPY --from=build /etc/ssl/certs /etc/ssl/certs
COPY --from=build /tmp/btcq-indexer/openapi/generated/doc.html ./openapi/generated/doc.html
COPY --from=build /tmp/btcq-indexer/dump .
COPY --from=build /tmp/btcq-indexer/btcq-indexer .
COPY --from=build /tmp/btcq-indexer/statechecks .
COPY --from=build /tmp/btcq-indexer/trimdb .
COPY --from=build /go/pkg/mod/github.com/!cosm!wasm/wasmvm/v2@v2.1.2/internal/api/libwasmvm.*.so /usr/lib
COPY config/config.json .
COPY resources /resources

CMD [ "./btcq-indexer", "config.json" ]
