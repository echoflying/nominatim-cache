# Deployment Hardening Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make the current version deployable on a production server with safer defaults, a working container image, runtime health checks, and clear operator documentation.

**Architecture:** Harden the existing Node.js service instead of redesigning it. Keep Express + SQLite + static frontend, but fix the production startup contract: deterministic env loading, explicit production credentials, self-contained Docker packaging, and a health endpoint that does not depend on upstream Nominatim availability.

**Tech Stack:** Node.js 20, TypeScript, Express, built-in `node:test`, Docker Compose, PM2, Nginx, SQLite

---

### Task 1: Add deployment safety tests

**Files:**
- Create: `server/tests/config-paths.test.mjs`
- Create: `server/tests/production-startup.test.mjs`
- Create: `server/tests/health-route.test.mjs`
- Modify: `server/package.json`

**Step 1: Write failing tests for env resolution, production startup, and health checks**

**Step 2: Run the targeted tests and confirm they fail for the expected reasons**

Run: `npm test -- --test-name-pattern "env|production|health"`

Expected: failures showing missing exports, missing `/health`, and production startup still allowing default credentials.

**Step 3: Add a repeatable test command**

Update `server/package.json` so `npm test` builds the service and runs the new Node test suite.

### Task 2: Harden runtime config and startup behavior

**Files:**
- Modify: `server/src/utils/config.ts`
- Modify: `server/src/index.ts`

**Step 1: Implement deterministic env file candidate resolution**

Support both `server/.env` and repository-root `.env`, while still allowing real environment variables to override dotenv values.

**Step 2: Enforce explicit admin credentials in production**

Fail startup when `NODE_ENV=production` and credentials are missing or still set to `admin/admin123`.

**Step 3: Add unauthenticated health endpoint**

Expose `/health` that reports process health without calling upstream services.

### Task 3: Fix deploy artifacts

**Files:**
- Modify: `server/Dockerfile`
- Modify: `docker-compose.yml`
- Create: `.dockerignore`
- Create: `server/src/scripts/migrate.ts`
- Modify: `server/ecosystem.config.js`
- Create: `server/logs/.gitkeep`

**Step 1: Convert Docker build to a self-contained multi-stage image**

Build with devDependencies in the builder stage, ship only production dependencies and built assets in the runtime stage, and include frontend assets in the image.

**Step 2: Update Compose health check to use `/health`**

Keep persistent data volume and remove dependence on external upstream availability.

**Step 3: Restore missing migration command target**

Add `server/src/scripts/migrate.ts` so `npm run db:migrate` is valid.

**Step 4: Make PM2 logs use a repo-local directory**

Avoid hidden server prerequisites like `/var/log/nominatim-cache`.

### Task 4: Remove insecure frontend default auth behavior

**Files:**
- Modify: `frontend/js/api.js`

**Step 1: Replace hard-coded default credentials with prompted credentials storage**

Prompt only when needed, retry once on 401, and keep the UI usable in production without shipping a known default password.

### Task 5: Document deployment and record changes

**Files:**
- Modify: `README.md`
- Create: `docs/DEPLOYMENT.md`
- Create: `docs/records/2026-03-28-deployment-hardening.md`

**Step 1: Document supported deployment modes**

Cover Docker Compose and PM2 + Nginx, env file locations, required variables, health checks, logs, and backup paths.

**Step 2: Record why these changes were made**

Summarize each production hardening change and the operational problem it addresses.

### Task 6: Verify end-to-end deployment readiness

**Files:**
- Modify as needed based on failures above

**Step 1: Run test suite**

Run: `npm test`

Expected: all tests pass.

**Step 2: Run production build**

Run: `npm run build`

Expected: TypeScript build succeeds.

**Step 3: Validate container configuration**

Run: `docker compose config`

Expected: compose renders successfully.
