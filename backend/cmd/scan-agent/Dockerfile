FROM golang:1.26-alpine AS builder

ARG TARGETARCH
# Keep proxy failure fallback consistent with the Web build.
ARG GOPROXY=https://goproxy.cn|https://proxy.golang.org|direct

WORKDIR /src/backend

RUN apk add --no-cache build-base git libpcap-dev sqlite-dev

COPY backend/ ./

ENV GOPROXY=${GOPROXY}

ARG VERSION=dev
ARG BUILD_TIME

RUN --mount=type=cache,id=hephaestus-agent-go-mod-${TARGETARCH},target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,id=hephaestus-agent-go-build-${TARGETARCH},target=/root/.cache/go-build,sharing=locked \
    go build \
    -trimpath \
    -ldflags="-linkmode external -extldflags '-static' -s -w -X main.Version=${VERSION} -X 'main.BuildTime=${BUILD_TIME}'" \
    -o /out/hephaestus-scan-agent \
    ./cmd/scan-agent

FROM scratch AS artifact

COPY --from=builder /out/hephaestus-scan-agent /hephaestus-scan-agent
