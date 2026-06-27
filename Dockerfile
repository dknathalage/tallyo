# syntax=docker/dockerfile:1
# Runtime-only image. The amd64 binary — with the SPA embedded via go:embed —
# is built on the host by `task image` and copied in. Nothing compiles here.
FROM gcr.io/distroless/static:nonroot AS final
COPY bin/tallyo-linux-amd64 /tallyo
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/tallyo"]
