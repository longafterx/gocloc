# syntax=docker/dockerfile:1
ARG GO_VERSION=1
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=devel
ARG REVISION=unknown

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath \
      -ldflags="-s -w -X main.Version=${VERSION} -X main.GitCommit=${REVISION}" \
      -o /out/gocloc ./cmd/gocloc

FROM scratch AS runtime
ARG VERSION=devel
ARG REVISION=unknown
LABEL org.opencontainers.image.title="gocloc" \
      org.opencontainers.image.description="A fast, parallel source code line counter" \
      org.opencontainers.image.source="https://github.com/longafterx/gocloc" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"
COPY LICENSE /LICENSE
WORKDIR /workdir
ENTRYPOINT ["/bin/gocloc"]

# Release CI supplies checksummed GoReleaser binaries instead of compiling again.
FROM runtime AS release
ARG TARGETARCH
COPY --chmod=0755 ${TARGETARCH}/gocloc /bin/gocloc

# The default target still supports building directly from the source checkout.
FROM runtime AS final
COPY --from=builder /out/gocloc /bin/gocloc
