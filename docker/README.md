# Docker Deployment

This guide covers deploying HealthOps using Docker and Docker Compose.

## Prerequisites

- Docker (20.10+)
- Docker Compose (2.0+)

## Quick Start

### 1. Build the Docker image

```bash
docker build -t healthops .
```

### 2. Start the stack

```bash
docker compose up -d
```

This starts:
- **healthops** service on port 8080 (Go backend + React frontend)
- **MongoDB** on port 27017

### 2b. (Optional) Start with demo services

To try HealthOps with realistic monitoring targets (nginx, MySQL, Redis, echo server, Linux servers):

```bash
docker compose -f docker-compose.yml -f docker-compose.demo.yml up -d
```

This adds:
- **nginx** on port 8081 (web server with health endpoint)
- **MySQL** on port 3306 (for MySQL deep monitoring)
- **Redis** on port 6379 (cache, TCP check)
- **echo-server** on port 9000 (simple API endpoint)
- **linux-server-1** on SSH port 2222 (nginx + python HTTP server + cron)
- **linux-server-2** on SSH port 2223 (nginx + python HTTP server + cron)

The Linux servers run real processes (nginx, python3 http.server, cron) that HealthOps monitors remotely via SSH process checks.

All demo services come pre-configured with health checks in `backend/config/demo.json`.

### 3. View logs

```bash
# Follow all logs
docker-compose logs -f

# Follow specific service logs
docker-compose logs -f healthops
docker-compose logs -f mongo
```

### 4. Verify deployment

```bash
# Check service health
curl http://localhost:8080/healthz

# View readiness status
curl http://localhost:8080/readyz

# Get health summary
curl http://localhost:8080/api/v1/summary
```

## Configuration

### Environment Variables

Create a `.env` file from the example:

```bash
cp .env.example .env
```

Edit `.env` to customize:

- **AUTH_ENABLED** - Enable/disable authentication (default: false in docker-compose.yml)
- **AUTH_USERNAME** - Admin username (default: admin)
- **AUTH_PASSWORD** - Admin password (default: admin123 - change in production!)
- **MONGODB_URI** - MongoDB connection string
- **MONGODB_DATABASE** - Database name (default: healthops)
- **STATE_PATH** - Path to local state file
- **CONFIG_PATH** - Path to config file

### Volume Mounts

The docker-compose.yml mounts:
- `./backend/data:/root/data` - Persistent state storage
- `./backend/config:/root/config` - Configuration files

This ensures your data and configuration survive container restarts.

## Management Commands

### Stop services

```bash
docker-compose down
```

### Stop and remove volumes

```bash
docker-compose down -v
```

### Restart services

```bash
docker-compose restart
```

### Rebuild after code changes

```bash
docker-compose up -d --build
```

### Execute commands in container

```bash
# Open shell in healthops container
docker-compose exec healthops sh

# View config
docker-compose exec healthops cat /root/config/default.json

# Check state
docker-compose exec healthops cat /root/data/state.json
```

## Production Considerations

1. **Change default credentials** - Update AUTH_PASSWORD in production
2. **Enable authentication** - Set AUTH_ENABLED=true
3. **Secure MongoDB** - Configure MongoDB authentication
4. **Resource limits** - Add memory/CPU limits to docker-compose.yml
5. **Log aggregation** - Configure centralized logging
6. **Health checks** - Use /healthz and /readyz for orchestration
7. **Backup strategy** - Regularly backup both data volume and MongoDB

## Troubleshooting

### Service fails to start

```bash
# Check logs
docker-compose logs healthops

# Verify config
docker-compose exec healthops cat /root/config/default.json
```

### MongoDB connection issues

```bash
# Check MongoDB is running
docker-compose ps

# Test MongoDB connection
docker-compose exec mongo mongosh --eval "db.adminCommand('ping')"
```

### Data persistence issues

```bash
# Check volume mounts
docker-compose exec healthops ls -la /root/data

# Verify data directory permissions
ls -la ./backend/data
```

## Network Access

By default, services are exposed on:
- **Health Monitor API**: http://localhost:8080
- **MongoDB**: mongodb://localhost:27017

For production deployment, consider:
- Using reverse proxy (nginx/traefik)
- Enabling HTTPS/TLS
- Restricting MongoDB to internal network only
- Configuring proper firewall rules

## Upgrading

1. Pull latest code
2. Rebuild image: `docker-compose build`
3. Restart with new image: `docker-compose up -d`
4. Monitor logs: `docker-compose logs -f`

The data volumes persist across upgrades.
