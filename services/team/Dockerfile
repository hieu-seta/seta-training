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

WORKDIR /src/services/team
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/team-svc ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/team-svc /team-svc
USER nonroot:nonroot
EXPOSE 8082
ENTRYPOINT ["/team-svc"]
