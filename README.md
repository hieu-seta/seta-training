# 🔥 Microservices Challenge: User, Team & Asset Management

## 🎯 Project Intention & Introduction
Welcome to the Capstone Mini-Project for the 2026 Golang Intern Course! 

The goal of this project is to evaluate your system design, coding, and problem-solving skills in building a robust backend system. Unlike standard tutorials, **this project is technology-agnostic.** While the core language is Go (to align with the course), you have the absolute freedom to choose your preferred API protocols (REST, GraphQL, gRPC), databases (SQL vs. NoSQL), frameworks, and infrastructure tools. 

You are expected to research your choices, justify your trade-offs, and implement best practices for a microservices architecture. The project will be released in **3 progressive stages**, allowing you to continuously build, iterate, and scale the system alongside the course syllabus.

> 💡 *"Every technical decision should be a deliberate trade-off, balancing the pragmatism of today with the scale of tomorrow."*

## 👩🏻‍💻 System Overview
You are tasked with building a microservices-based system to manage users, teams, and digital assets. 
* **Users** can have varying roles (Manager or Member).
* **Managers** can form teams and manage personnel.
* **All Users** can manage, organize, and share digital assets (folders & notes) with granular access control.

## 📋 General Requirements

### 1. General Engineering Standards
* **Authentication & Authorization:** Secure all endpoints and validate user roles before executing sensitive actions.
* **Clean Architecture:** Structure your code utilizing separation of concerns (e.g., handlers, services, repositories).
* **Error Handling:** Ensure graceful degradation and use appropriate HTTP/RPC status codes.

### 2. Technical Documentation
* You must prepare a **500–800 word document** describing the tech stack used in your project.
* Detail why you chose your specific database, message queue, or API protocol. What were the trade-offs, and how do they benefit your specific implementation?

### 3. Final Presentation (Demo & Future Pitch)
* You will participate in a live **3-minute presentation**.
* Pitch **one major future improvement** for your system. If this were a real startup, what is the next technical bottleneck you would hit, and how would you re-architect the system to solve it?

*(Note: The detailed technical specifications for the project are divided into Stage 1, Stage 2, and Stage 3 documents. All submissions must be in English).*

---

## 🚀 My Submission — Quick Start

```bash
cp .env.example .env       # placeholders are fine for local
make up                    # boots PG + Redis + NATS + 3 svc + audit worker + Loki/Promtail/Grafana
make e2e                   # runs every phase e2e script (43 checks)
```

- `auth-svc`   → http://localhost:8081
- `team-svc`   → http://localhost:8082
- `asset-svc`  → http://localhost:8083
- `grafana`    → http://localhost:3001  (admin/admin, Loki datasource pre-wired)
- `nats /jsz`  → http://localhost:8222/jsz

## 📚 Docs

| Doc | Purpose |
|---|---|
| [SOLUTION.md](SOLUTION.md) | Stack picks + trade-offs (one-page) |
| [docs/TECH_STACK.md](docs/TECH_STACK.md) | 500-800 word tech-stack doc (required by §2) |
| [docs/DEMO_SCRIPT.md](docs/DEMO_SCRIPT.md) | 3-minute live demo outline |
| [docs/FUTURE.md](docs/FUTURE.md) | Future-bottleneck pitch — CQRS ACL read model |
| [plans/](plans/20260521-1114-microservices-impl-and-test/plan.md) | 10-phase implementation plan + per-phase reviews |

## 🏗 Architecture (live)

```
                 ┌─────────┐
client ────▶     │  Gin    │  :8081 / :8082 / :8083
                 └────┬────┘
       JWT HS256 (validated locally)
                      │
   ┌──────────────────┼─────────────────┬───────────────┐
   ▼                  ▼                 ▼               ▼
 auth-svc         team-svc          asset-svc      audit-worker
   │                │ ┘  │             │ ┘ │           ▲
   └── Postgres ────┘    │             │   │           │
         (schema/svc)    │             │   │           │
                         └── Redis ────┘   │           │
                              (cache)      │           │
                                           └──NATS─────┘
                                            (events)
                            ┌────────┐
       Promtail → Loki ◀───┤ stdout │  (all svc, JSON, X-Request-Id)
                            └────────┘
                                ▼
                            Grafana
```

## 🧪 Test Layers

| Layer | Where | What |
|---|---|---|
| Unit | `pkg/*` + `services/*/internal/service` | jwtauth, ACL, csv-import workers, audit dedup, cache wrapper, req-id |
| Integration | `*_integration_test.go` (build tag `integration`) | tc-go PG + Redis + NATS; UserRepo, TeamRepo, AssetRepo UNION, events round-trip + dedup |
| Handler | `services/*/internal/handler/*_test.go` | full Gin router via `ServeHTTP`, fake repos w/ JWT generation |
| E2E | `scripts/e2e/0*-*.sh` | bash + curl + jq against live compose stack — 8 scripts, 43 assertions total |

## ⚙️ Useful targets

```
make up       # start everything
make down     # stop everything
make logs     # tail compose logs
make lint     # golangci-lint v2 across all modules
make test     # unit tests (race + short)
make test-int # integration tests (race + tag=integration)
make build    # compile all svc binaries
make e2e      # run all phase e2e scripts
```

