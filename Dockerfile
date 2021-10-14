FROM --platform=$BUILDPLATFORM golang:1.16 as builder
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ENV GOOS=$TARGETOS GOARCH=$TARGETARCH
SHELL ["/bin/bash", "-c"]
WORKDIR /go/src/github.com/virtual-kubelet/azure-aci
COPY go.mod ./
COPY go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,id=vk-azure-aci,sharing=locked,target=/root/.cache/go-build \
    GOARM="${TARGETVARIANT#v}" make build GOARM="$GOARM"

FROM --platform=$TARGETPLATFORM scratch
COPY --from=builder /go/src/github.com/virtual-kubelet/azure-aci/bin/virtual-kubelet /usr/bin/virtual-kubelet
COPY --from=builder /etc/ssl/certs/ /etc/ssl/certs
ENTRYPOINT [ "/usr/bin/virtual-kubelet" ]
CMD [ "--help" ]
