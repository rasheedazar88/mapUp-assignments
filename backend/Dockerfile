# ---- Stage 1: build a static Go binary ----
FROM golang:1.27-alpine AS build
WORKDIR /src

# Download dependencies first (cached layer while go.mod/go.sum are unchanged).
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO off → fully static binary that runs on a minimal base image.
# -trimpath and -s -w make it smaller and free of local paths.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---- Stage 2: tiny runtime image (no shell, no compiler, non-root) ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
