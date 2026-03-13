# LinkTracker

A lightweight Go server that tracks clicks from multiple sharing channels, records visitor data, and redirects users to the target URL — all in real time. Includes a live dashboard to monitor results.

---

## Features

- **3 tracking endpoints** — separate links for LinkedIn, WhatsApp, and Community groups
- **Captures** IP address, timestamp, and user agent per click
- **Instant redirect** — visitors land on the destination with zero friction
- **Live dashboard** — bar chart + click log, auto-refreshes every 30 s
- **PostgreSQL** persistence — survives restarts, queryable at any time
- **Proxy-aware** — reads `X-Forwarded-For` / `X-Real-IP` headers correctly behind Nginx/Cloudflare

---

## Architecture

```
Browser
  │
  ├─ GET /linkedin   ─┐
  ├─ GET /whatsapp   ─┤──► record click in Postgres ──► 302 Redirect to target URL
  └─ GET /community  ─┘

  GET /             ──► Dashboard (static HTML + Chart.js)
  GET /api/stats    ──► JSON: per-source totals
  GET /api/clicks   ──► JSON: last 100 click records
```

---

## Quick Start (Docker — recommended)

**Prerequisites:** Docker + Docker Compose installed.

```bash
git clone <your-repo-url>
cd LinkTracker
docker compose up --build
```

Open **http://localhost:8080** — the dashboard is live.

Your tracking links:

| Channel   | URL to share                    |
|-----------|---------------------------------|
| LinkedIn  | `http://your-server/linkedin`   |
| WhatsApp  | `http://your-server/whatsapp`   |
| Community | `http://your-server/community`  |

---

## Quick Start (Local / without Docker)

**Prerequisites:** Go 1.21+, PostgreSQL running locally.

```bash
# 1. Clone and enter the project
git clone <your-repo-url>
cd LinkTracker

# 2. Configure environment
cp .env.example .env
# Edit .env if your Postgres credentials differ

# 3. Export variables and run
export $(cat .env | xargs)
go run .
```

The server starts on port `8080` by default. The `clicks` table is created automatically on first run.

---

## Environment Variables

| Variable       | Default                                                          | Description                    |
|----------------|------------------------------------------------------------------|--------------------------------|
| `DATABASE_URL` | `postgres://postgres:password@localhost:5432/linktracker?sslmode=disable` | PostgreSQL connection string   |
| `PORT`         | `8080`                                                           | HTTP port the server listens on |

---

## API Reference

### `GET /api/stats`

Returns total clicks grouped by source.

```json
[
  { "source": "linkedin",  "count": 42 },
  { "source": "whatsapp",  "count": 17 },
  { "source": "community", "count": 8  }
]
```

### `GET /api/clicks`

Returns the 100 most recent click records, newest first.

```json
[
  {
    "id": 67,
    "source": "linkedin",
    "ip_address": "197.x.x.x",
    "user_agent": "Mozilla/5.0 ...",
    "created_at": "2026-03-13T14:22:01Z"
  },
  ...
]
```

---

## Database Schema

```sql
CREATE TABLE clicks (
    id         SERIAL PRIMARY KEY,
    source     VARCHAR(50)  NOT NULL,          -- "linkedin" | "whatsapp" | "community"
    ip_address VARCHAR(100) NOT NULL,
    user_agent TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

---

## Project Structure

```
LinkTracker/
├── main.go              # Go server — handlers, DB init, routing
├── go.mod               # Module definition
├── go.sum               # Dependency checksums
├── Dockerfile           # Multi-stage build (builder + alpine runtime)
├── docker-compose.yml   # App + Postgres, one command startup
├── .env.example         # Environment variable template
└── static/
    └── index.html       # Dashboard — summary cards, bar chart, click table
```

---

## Dashboard Preview

The dashboard at `/` shows:

- **Summary cards** — click counts for each channel plus a grand total
- **Bar chart** — visual comparison across channels (Chart.js)
- **Click log** — scrollable table with source badge, IP, timestamp (UTC), and user agent
- **Auto-refresh** every 30 seconds — no manual reload needed

---

## Deployment Notes

- Put the server behind **Nginx or Caddy** for TLS (HTTPS). Use your domain so sharing links look clean, e.g. `https://track.yourdomain.com/linkedin`.
- If using Nginx as a reverse proxy, add these headers so IP detection works correctly:
  ```nginx
  proxy_set_header X-Forwarded-For $remote_addr;
  proxy_set_header X-Real-IP       $remote_addr;
  ```
- The Postgres `pgdata` Docker volume persists data across container restarts.

---

## Tech Stack

| Layer     | Technology                  |
|-----------|-----------------------------|
| Server    | Go 1.21, `net/http`         |
| Database  | PostgreSQL 16               |
| DB driver | `github.com/lib/pq`         |
| Frontend  | Vanilla HTML/CSS/JS         |
| Charts    | Chart.js 4 (CDN)            |
| Container | Docker + Docker Compose     |
