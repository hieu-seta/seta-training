# syntax=docker/dockerfile:1
# Multi-stage build for auth-svc.

FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /src

# Copy workspace + all referenced modules (cmd/migrator is referenced by go.work).
COPY go.work ./
COPY pkg/ ./pkg/
COPY tools/migrator/ ./tools/migrator/
COPY services/auth/ ./services/auth/
COPY services/team/ ./services/team/
COPY services/asset/ ./services/asset/
COPY services/audit-worker/ ./services/audit-worker/

WORKDIR /src/services/auth
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/auth-svc ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/auth-svc /auth-svc
USER nonroot:nonroot
EXPOSE 8081
ENTRYPOINT ["/auth-svc"]
