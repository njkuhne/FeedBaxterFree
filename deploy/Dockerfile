# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM node:22-alpine AS ui
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
COPY --from=ui /web/dist ./web/dist
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-buildvcs=false GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X github.com/njkuhne/feedbaxter.Version=${VERSION}" \
    -o /out/feedbaxter ./cmd/feedbaxter
RUN mkdir -p /out/data && chown 65532:65532 /out/data

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/feedbaxter /feedbaxter
COPY --from=build --chown=65532:65532 /out/data /data
ENV DATA_DIR=/data
USER 65532:65532
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/feedbaxter"]
CMD ["serve"]
