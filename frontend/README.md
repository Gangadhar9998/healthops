# HealthOps Frontend

Modern React dashboard for the HealthOps monitoring platform.

## Tech Stack

- **React 19** with TypeScript 5.7
- **Vite 6** — fast dev server and bundler
- **Tailwind CSS** — utility-first styling
- **TanStack React Query** — server state management with caching
- **Recharts** — uptime, response time, and analytics charts
- **React Router 7** — client-side routing
- **Lucide React** — icon system

## Pages

| Page | Route | Description |
|------|-------|-------------|
| Dashboard | `/` | Health summary, uptime %, active incidents, response time chart |
| Servers | `/servers` | Server inventory, add/edit/delete servers |
| Checks | `/checks` | All health checks with status, type filter, search |
| Incidents | `/incidents` | Incident list with severity, timeline, acknowledge/resolve |
| MySQL | `/mysql` | MySQL monitoring: connections, queries, threads, server stats |
| Analytics | `/analytics` | Uptime by check, response times, failure rates, MTTA/MTTR |
| AI Analysis | `/ai` | AI provider status, recent analyses, manual analysis trigger |
| Settings | `/settings` | General config, servers, health checks, alert rules, AI providers, export |

## Development

```bash
# Install dependencies
npm install

# Start dev server (proxies API to backend on :8080)
npm run dev

# Build for production
npm run build

# Preview production build
npm run preview

# Lint
npm run lint
```

The dev server runs on `http://localhost:5173` and proxies `/api` requests to `http://localhost:8080`.

## Key Features

- **Real-time updates** via SSE (Server-Sent Events) — dashboard updates live without polling
- **Dark mode** toggle in the header
- **CSV/JSON export** for incidents and check results
- **Responsive layout** with collapsible sidebar navigation
- **Error boundaries** and loading states throughout
