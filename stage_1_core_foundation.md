# 🚀 Stage 1: The Core Foundation (Identity & Teams)

*Focus: API design, Database Modeling, Authentication, and basic Microservices communication.*

## 1. Auth & User Management Service
**Responsibilities:** Handle user registration, login, token/session generation, and role assignment.
* **Entities:** User
    * `userId` (auto-generated)
    * `username`
    * `email` (unique)
    * `role`: "manager" or "member"
* **Functional Requirements:**
    * Create a user, login, logout, and fetch user lists.
    * Passwords must be securely hashed. 
    * Generate secure tokens (e.g., JWT) for session management.
    * Define roles strictly at creation.

## 2. Team Management Service
**Responsibilities:** Allow managers to create teams and manage members.
* **Entities:** Team
    * `teamId`
    * `teamName`
    * `managers` (list of users)
    * `members` (list of users)
* **Functional Requirements:**
    * Managers can create teams, add/remove members, and add/remove other managers (only the main manager can do this).
    * **Rules:** Managers can only manage users within their own teams. Members cannot create or manage teams.
