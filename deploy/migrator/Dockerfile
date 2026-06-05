# syntax=docker/dockerfile:1
# One-shot migration runner.
FROM migrate/migrate:v4.18.1
COPY migrations /migrations
ENTRYPOINT ["migrate"]
