# syntax=docker/dockerfile:1

# Stage 1: build the SvelteKit SPA (emits web/build). SPA output is JS/HTML —
# architecture-independent — so build on the native BUILDPLATFORM (no emulation).
FROM --platform=$BUILDPLATFORM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: build the cgo-free Go binary with the SPA embedded. Runs on the
# native BUILDPLATFORM and CROSS-COMPILES to the requested TARGETARCH (Go does
# this trivially without cgo), so `--platform linux/amd64` for Cloud Run does
# not pay QEMU emulation cost.
FROM --platform=$BUILDPLATFORM golang:1.26 AS build
ARG TARGETOS=linux
ARG TARGETARCH=amd64
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/build ./web/build
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -ldflags="-s -w" -o /tallyo ./cmd/tallyo

# Stage 3: minimal distroless runtime, tagged for the TARGET platform.
FROM gcr.io/distroless/static:nonroot AS final
COPY --from=build /tallyo /tallyo
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/tallyo"]
