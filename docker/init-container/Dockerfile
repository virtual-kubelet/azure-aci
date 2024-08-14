FROM --platform=$BUILDPLATFORM golang:1.22 as builder
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
ENV GOCACHE=/root/gocache
RUN \
    --mount=type=cache,target=${GOCACHE} \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the go source
COPY cmd/init-container/main.go main.go
COPY . .

RUN --mount=type=cache,target=${GOCACHE} \
    --mount=type=cache,id=vk-azure-aci-init,sharing=locked,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} GO111MODULE=on go build -o initcontainer main.go

FROM --platform=$BUILDPLATFORM gcr.io/distroless/static
WORKDIR /
COPY --from=builder  /workspace/initcontainer .

ENTRYPOINT ["/initcontainer"]
