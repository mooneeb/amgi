# ── Stage 1: builder ──────────────────────────────────────────────
# Alpine-based Go image — smaller than debian, sufficient for builds.
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Copy go.mod/go.sum FIRST, before source.
# Docker caches each layer. If neither file changes between builds,
# `go mod download` is reused from cache. Otherwise every source edit
# re-downloads every dependency. Non-trivial time savings.
COPY go.mod go.sum ./
RUN go mod download

# Now copy the rest of the source.
COPY . .

# Build a fully-static binary.
#   CGO_ENABLED=0 : disable cgo → statically linked, no libc needed
#   GOOS=linux    : explicit target (belt-and-suspenders for non-linux dev machines)
#   -ldflags "-w -s" : strip DWARF debug info + symbol table → smaller binary
#                    (optional but common for release builds)
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s" \
    -o /amgi \
    ./cmd/amgi

# ── Stage 2: runtime ──────────────────────────────────────────────
# distroless/static = minimal base with CA certs + /etc/passwd + /etc/group.
# No shell, no package manager, no extras. ~2MB.
# `nonroot` variant runs as UID 65532 by default.
FROM gcr.io/distroless/static-debian12:nonroot

# Copy just the binary from the builder stage.
COPY --from=builder /amgi /amgi

# Default Paths. Can be overridden at runtime with -e.
ENV CONFIG_PATH=/etc/amgi/config.yaml
ENV AMGI_DB_PATH=/etc/amgi/amgi.db

# Documentation only (does NOT open the port).
# Actual binding happens at `docker run -p 8080:8080 ...`
EXPOSE 8080

ENTRYPOINT ["/amgi"]