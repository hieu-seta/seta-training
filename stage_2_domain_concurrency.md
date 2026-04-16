# 🚀 Stage 2: The Domain & Concurrency (Assets & Processing)

*Focus: Complex business logic, Authorization (RBAC), and Go Concurrency patterns.*

## 1. Asset Management & Sharing Service
**Responsibilities:** Manage the creation and sharing of digital assets across users and teams.
* **Entities:** * **Folders:** Owned by users.
    * **Notes:** Belong to folders, contain text content.
* **Functional Requirements:**
    * Users can create, read, update, and delete folders and notes.
    * **Sharing:** Users can share folders or individual notes with other users (Read or Write access) and revoke access at any time.
    * **Inheritance:** When sharing a folder, all notes inside are automatically shared.
    * **Manager Oversight:** Managers have default "Read-Only" access to all assets owned by their team members, but cannot edit them unless explicitly shared with write access.

## 2. The Concurrency Challenge: Bulk User Import
**Responsibilities:** Handle heavy data processing efficiently without blocking the main application thread.
* **Task:** Create an endpoint (`POST /import-users`) that accepts a `.csv` file containing user data.
* **Execution:** Parse the file and create users concurrently using Go routines, channels, and a worker pool.
* **Output:** Return a processing summary (e.g., 50 succeeded, 3 failed).
