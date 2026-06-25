# tonberry — official ESL (Eidolons Spec Lifecycle) MCP.
# Thin 2-stage build: a golang:1.23 builder produces a fully-static CGO-off
# binary; the runtime layer is gcr.io/distroless/static-debian12:nonroot
# (CA certs + a nonroot user, a few MB). Matches the Junction thin-Go precedent.

# ---- builder ----
FROM golang:1.23 AS builder
WORKDIR /src

# Module graph first for layer caching.
COPY go.mod go.sum ./
RUN go mod download

# Source.
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Fully-static, stripped, reproducible-ish binary.
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -trimpath -o /out/tonberry ./cmd/tonberry

# ---- runtime ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/tonberry /tonberry
# Default workdir is the mounted project tree (the .mcp.json template mounts
# __PROJECT_ROOT__ at /workspace); tonberry reads/writes .spectra/changes/ there.
WORKDIR /workspace
ENTRYPOINT ["/tonberry"]
CMD ["serve"]
