FROM --platform=$BUILDPLATFORM golang:1.18 as builder
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ENV GOOS=$TARGETOS GOARCH=$TARGETARCH
ARG VERSION_FLAGS
SHELL ["/bin/bash", "-c"]
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

COPY . .

RUN --mount=type=cache,target=${GOCACHE} \
    --mount=type=cache,id=vk-azure-aci,sharing=locked,target=/go/pkg/mod \
     CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -ldflags '-extldflags "-static"' \
     "${VERSION_FLAGS}" -o bin/virtual-kubelet cmd/virtual-kubelet/*

FROM --platform=$BUILDPLATFORM gcr.io/distroless/static
COPY --from=builder  /workspace/bin/virtual-kubelet /usr/bin/virtual-kubelet
COPY --from=builder /etc/ssl/certs/ /etc/ssl/certs

ENTRYPOINT [ "/usr/bin/virtual-kubelet" ]
CMD [ "--help" ]
