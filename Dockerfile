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

# Pre-create an empty data directory in the builder stage so it can be
# copied into the runtime image with the correct ownership. Docker's
# volume initialization copies the IMAGE's path metadata into freshly-
# mounted named volumes — without this, fresh volumes default to
# root:root ownership and AMGI (UID 65532) cannot write its SQLite DB.
# Distroless has no shell or `chown`, so the directory must be
# prepared here in the builder stage.
RUN mkdir -p /opt/amgi-data

# ── Stage 2: runtime ──────────────────────────────────────────────
# distroless/static = minimal base with CA certs + /etc/passwd + /etc/group.
# No shell, no package manager, no extras. ~2MB.
# `nonroot` variant runs as UID 65532 by default.
FROM gcr.io/distroless/static-debian12:nonroot

# Copy just the binary from the builder stage.
COPY --from=builder /amgi /amgi

# Copy the prepared data directory into the runtime image with
# UID:GID 65532:65532 ownership. Fresh named volumes mounted at this
# path will inherit this ownership, allowing the nonroot container
# user to write its SQLite database without operator intervention.
COPY --from=builder --chown=65532:65532 /opt/amgi-data /var/lib/amgi

# Default Paths. Can be overridden at runtime with -e.
ENV CONFIG_PATH=/etc/amgi/config.yaml
ENV AMGI_DB_PATH=/var/lib/amgi/amgi.db

# Documentation only (does NOT open the port).
# Actual binding happens at `docker run -p 8080:8080 ...`
EXPOSE 8080

ENTRYPOINT ["/amgi"]