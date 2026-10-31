# syntax=docker/dockerfile:1

# ---- build stage ------------------------------------------------------------
# $BUILDPLATFORM + TARGETOS/TARGETARCH: the Go compiler runs natively on the
# build host (fast on Apple Silicon) and cross-compiles for the target
# (linux/amd64 on Railway) instead of emulating the whole toolchain under QEMU.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src

# Dependencies first: this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
# Static, stripped, reproducible binary.
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/arafa-360 .

# ---- runtime stage ----------------------------------------------------------
# distroless/static: no shell, no package manager, ships CA certs + tzdata
# (needed for TLS to Sentry / managed Postgres / Redis). ~2 MB base.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/arafa-360 /arafa-360

ENV APP_ENV=production
# Railway injects PORT at runtime; EXPOSE is documentation only.
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/arafa-360"]
