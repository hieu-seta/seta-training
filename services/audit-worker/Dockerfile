# syntax=docker/dockerfile:1
FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src

COPY go.work ./
COPY pkg/ ./pkg/
COPY tools/migrator/ ./tools/migrator/
COPY services/auth/ ./services/auth/
COPY services/team/ ./services/team/
COPY services/asset/ ./services/asset/
COPY services/audit-worker/ ./services/audit-worker/

WORKDIR /src/services/audit-worker
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/audit-worker ./cmd/worker

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/audit-worker /audit-worker
USER nonroot:nonroot
ENTRYPOINT ["/audit-worker"]
