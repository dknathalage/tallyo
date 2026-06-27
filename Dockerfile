# syntax=docker/dockerfile:1

# Stage 1: build the SvelteKit SPA (emits web/build).
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: build the cgo-free Go binary with the SPA embedded.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/build ./web/build
ENV CGO_ENABLED=0 GOOS=linux GOFLAGS=-trimpath
RUN go build -ldflags="-s -w" -o /tallyo ./cmd/tallyo

# Stage 3: minimal distroless runtime.
FROM gcr.io/distroless/static:nonroot AS final
COPY --from=build /tallyo /tallyo
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/tallyo"]
