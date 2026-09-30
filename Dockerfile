# syntax=docker/dockerfile:1
# Pimpo in a container: for a home server or a VPS. The data lives in the
# /data volume; the web app and the API listen on port 7788.
#
#   docker run -d --name pimpo -p 127.0.0.1:7788:7788 -v pimpo:/data ghcr.io/turbine-dev/pimpo
#   docker logs pimpo | grep auth      # the login link
#
# See docs/SELF_HOSTING.md.
#
# Base images are pinned by digest (the tag is kept for reading); update
# both together.

FROM --platform=$BUILDPLATFORM node:24-bookworm-slim@sha256:0e0ff40c39bc087845bfb27465a0df4ea419520094bc35842ff83dd8cbe6f9b6 AS ui
WORKDIR /src/ui
COPY ui/package.json ui/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY ui/ ./
RUN npx vite build

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm@sha256:a688600ca24f8a4d3ca77f95b0dd40704a9fc787c826660eb7ba0b641b8b175d AS server
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /src/internal/server/dist internal/server/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/pimpo ./cmd/pimpo

FROM debian:bookworm-slim@sha256:3783cc01769c7b2b1b83a5c5ad96c815348e28ed7da68e2e3687004faa906251
# ca-certificates for https, tzdata for routines' local times, curl for
# the health check.
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata curl && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 10001 --create-home --home-dir /home/pimpo pimpo \
 && mkdir /data && chown pimpo:pimpo /data
COPY --from=server /out/pimpo /usr/local/bin/pimpo
USER pimpo
ENV PIMPO_HOME=/data
VOLUME /data
EXPOSE 7788
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s CMD curl -fsS http://127.0.0.1:7788/api/health || exit 1
# Inside the container Pimpo listens on every interface; publish the port
# on 127.0.0.1 or behind https (Tailscale, a reverse proxy), never openly.
ENTRYPOINT ["pimpo"]
CMD ["serve", "--addr", "0.0.0.0:7788"]
