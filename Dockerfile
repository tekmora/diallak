# Build stage. It always runs on the platform of the machine that builds, and Go compiles for
# the platform the image is for (TARGETOS, TARGETARCH, set by BuildKit). An image for another
# architecture therefore needs no emulation.
FROM --platform=$BUILDPLATFORM golang:1.27.4-trixie@sha256:f18c3d73a184fb0582828510722e723b63a93f7188d11dd223d9be6f6880e65d AS builder
WORKDIR /src
COPY go.mod go.sum* ./
# The builder runs the toolchain go.mod names, or the build stops. The image sets
# GOTOOLCHAIN=local, so a builder with another Go version would otherwise build the release
# with it and say nothing.
RUN set -eu; \
    want="$(sed -n 's/^toolchain //p' go.mod)"; \
    have="$(go env GOVERSION)"; \
    if [ -z "$want" ] || [ "$want" != "$have" ]; then \
      echo "builder runs ${have}, go.mod names toolchain ${want:-none}" >&2; \
      exit 1; \
    fi
RUN go mod download
COPY . .
# The main package the image runs. The module holds a second main package, the secrets gate
# (tools/secrets/gate), so the default names the bot. Set empty, the build takes the module's
# only main package and stops naming what it found when there is none or more than one.
# Choose another with: docker build --build-arg MAIN_PACKAGE=./cmd/<name> .
ARG MAIN_PACKAGE=./cmd/diallak
ARG TARGETOS=linux
ARG TARGETARCH
RUN set -eu; \
    pkg="${MAIN_PACKAGE:-}"; \
    if [ -z "$pkg" ]; then \
      listed="$(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./...)"; \
      mains=0; names=""; \
      for candidate in $listed; do mains=$((mains + 1)); pkg="$candidate"; names="$names $candidate"; done; \
      if [ "$mains" -ne 1 ]; then \
        echo "MAIN_PACKAGE is unset and the module has $mains main packages:${names:- none}" >&2; \
        exit 1; \
      fi; \
    fi; \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
      go build -trimpath -ldflags="-s -w" -o /bin/app "$pkg"

# Distroless runtime
FROM gcr.io/distroless/static-debian13:nonroot@sha256:aafe993665bf6200db38a02c11a9351e782a84b6a45e7f33b6695638c88e3931
WORKDIR /
COPY --from=builder /bin/app /app
USER 65532:65532
ENTRYPOINT ["/app"]
