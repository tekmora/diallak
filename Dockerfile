# Build stage. It always runs on the platform of the machine that builds, and Go compiles for
# the platform the image is for (TARGETOS, TARGETARCH, set by BuildKit). An image for another
# architecture therefore needs no emulation.
FROM --platform=$BUILDPLATFORM golang:1.27.3-trixie@sha256:604106102d33b1084e37ca08adf98ac0813624ae8595bd0d7f3965ecebded09c AS builder
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
# The main package the image runs. Left empty, the build takes the module's only main
# package, wherever it lives, and stops naming what it found when there is none or more than
# one. Choose one with: docker build --build-arg MAIN_PACKAGE=./cmd/<name> .
ARG MAIN_PACKAGE=
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
FROM gcr.io/distroless/static-debian13:nonroot@sha256:e2e927ec666bae08560abb3c55d0659eceabb657f56b6782ab500a9fc7f555e3
WORKDIR /
COPY --from=builder /bin/app /app
USER 65532:65532
ENTRYPOINT ["/app"]
