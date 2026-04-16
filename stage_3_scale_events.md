# 🚀 Stage 3: Scale & Event-Driven Architecture

*Focus: System performance, Message Brokers, Caching, and DevOps.*

## 1. Event-Driven Communication
Integrate a Message Broker (e.g., Kafka, RabbitMQ) to decouple your services and create an event stream.
* **Team Events (Topic: `team.activity`):** * Emit events when teams are created or members/managers are added/removed (`TEAM_CREATED`, `MEMBER_ADDED`, etc.).
* **Asset Events (Topic: `asset.changes`):** * Emit events when assets are created, updated, deleted, or shared (`FOLDER_SHARED`, `NOTE_UPDATED`, etc.).
* **Consumers:** Build consumer workers to listen to these events. For example, log them to a separate database for an audit trail or trigger notifications.

## 2. High-Performance Caching
Integrate an in-memory data store (e.g., Redis) to optimize performance.
* **Real-Time Team Cache:** Cache the list of team members to prevent repeated database hits. Invalidate/update on `team.activity` events.
* **Asset Metadata Cache:** Cache folder/note details. Use a write-through strategy based on `asset.changes`.
* **Access Control Lookup:** Cache user permissions (`asset:{assetId}:acl`) to quickly validate if a user has access before querying the main database.

## 3. Observability & Deployment
* **Logging:** Implement centralized logging (e.g., Loki + Promtail) to track system health and errors across your microservices.
* **Containerization:** Write `Dockerfile`s for your services and a `docker-compose.yml` to spin up your entire infrastructure (App, DB, Cache, Broker) with a single command.
