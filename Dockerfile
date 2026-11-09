# syntax=docker/dockerfile:1

ARG FRONTEND_SOURCE=frontend-builder

FROM --platform=$BUILDPLATFORM node:22-alpine AS frontend-builder

WORKDIR /src/frontend

COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/ ./

ENV NEXT_TELEMETRY_DISABLED=1
ENV STATIC_EXPORT=true

RUN npm run build

# Release scripts already export the frontend on the host. A named context
# supplies those architecture-independent files without rebuilding Next.js.
FROM scratch AS frontend-prebuilt
COPY --from=frontend-dist / /src/frontend/out/

FROM ${FRONTEND_SOURCE} AS frontend-assets

FROM golang:1.26-alpine AS backend-builder

ARG TARGETARCH
ARG GOPROXY=https://goproxy.cn|https://proxy.golang.org|direct
ARG GO_BUILD_PARALLELISM=2

WORKDIR /src/backend

RUN apk add --no-cache build-base git libpcap-dev sqlite-dev

COPY backend/ ./
COPY --from=frontend-assets /src/frontend/out /src/backend/webassets/dist

ENV CGO_ENABLED=1 \
    GOPROXY=${GOPROXY}

ARG VERSION=dev
ARG BUILD_TIME=unknown

RUN --mount=type=cache,id=hephaestus-go-mod-${TARGETARCH},target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,id=hephaestus-go-build-${TARGETARCH},target=/root/.cache/go-build,sharing=locked \
    GOMAXPROCS=${GO_BUILD_PARALLELISM} go build -p "${GO_BUILD_PARALLELISM}" -ldflags="-linkmode external -extldflags '-static' -s -w -X main.Version=${VERSION} -X 'main.BuildTime=${BUILD_TIME}'" -o /out/hephaestus-web .

FROM scratch AS backend-artifact

COPY --from=backend-builder /out/hephaestus-web /hephaestus-web

# Single runtime image: PostgreSQL is embedded by default, so `docker run` needs
# no external database. Set HEPHAESTUS_DB_TYPE=sqlite or an external
# HEPHAESTUS_DB_HOST to opt out.
FROM alpine:3.22

RUN apk add --no-cache \
        ca-certificates \
        libpcap \
        sqlite-libs \
        tzdata \
        postgresql17 \
        postgresql17-client \
        python3 \
        py3-pip \
        py3-requests \
        bubblewrap \
        su-exec \
        bash \
        curl \
        wget \
        git \
        jq \
        unzip \
        nmap \
        bind-tools \
        netcat-openbsd \
        whois \
    && python3 -m pip install --no-cache-dir --break-system-packages --root-user-action=ignore \
        "sqlmap==1.10.10" \
    && rm -rf /root/.cache/pip

WORKDIR /opt/hephaestus

COPY --from=backend-builder /out/hephaestus-web /opt/hephaestus/hephaestus-web
COPY docker/entrypoint.sh /usr/local/bin/hephaestus-entrypoint.sh

RUN chmod +x /usr/local/bin/hephaestus-entrypoint.sh && \
    mkdir -p /var/lib/hephaestus && \
    chmod 755 /var/lib/hephaestus

ENV GIN_MODE=release
ENV PORT=18181
ENV TZ=Asia/Shanghai
ENV HEPHAESTUS_CONFIG_DIR=/var/lib/hephaestus
ENV HEPHAESTUS_RESOURCE_DIR=/var/lib/hephaestus/config
ENV HEPHAESTUS_DB_TYPE=postgres
ENV HEPHAESTUS_DB_HOST=local
ENV HEPHAESTUS_DB_PORT=5432
ENV HEPHAESTUS_DB_USER=postgres
ENV HEPHAESTUS_DB_NAME=hephaestus
ENV HEPHAESTUS_DEPLOYMENT=docker
ENV PYTHONUNBUFFERED=1

VOLUME ["/var/lib/hephaestus"]

EXPOSE 18181

STOPSIGNAL SIGTERM

HEALTHCHECK --interval=30s --timeout=5s --start-period=120s --retries=5 \
    CMD wget -qO- "http://127.0.0.1:${PORT}/healthz" >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/hephaestus-entrypoint.sh"]
