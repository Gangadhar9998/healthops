#!/bin/bash
set -e

# Start SSH daemon
/usr/sbin/sshd

# Start cron daemon
cron

# Start nginx in background
nginx &

# Start Python HTTP server on configured port (default 8000)
HTTP_PORT=${HTTP_PORT:-8000}
nohup python3 -m http.server "$HTTP_PORT" --directory /tmp > /dev/null 2>&1 &

echo "=== Linux Demo Server Started ==="
echo "  SSH:    port 22"
echo "  Nginx:  port 80"
echo "  Python: port $HTTP_PORT"
echo "  Cron:   running"
echo "================================="

# Keep container running
exec tail -f /dev/null
