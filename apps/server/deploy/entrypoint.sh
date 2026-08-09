#!/bin/sh
set -eu

# An explicit command (e.g. `docker run … checks`) runs the binary directly — a
# one-shot pass shouldn't also spin up the cron daemon.
if [ "$#" -gt 0 ]; then
	exec /app/preuzmi "$@"
fi

# Default (no args): start busybox cron for the built-in daily receipt check
# (see deploy/crontab). crond's own messages go to this process's stderr; each
# job redirects its output to PID 1's stdout, so everything lands in `docker
# logs`.
crond -b -l 8 -L /dev/stderr -c /etc/crontabs

# Web server in the foreground so the container's lifecycle follows it: if it
# exits, the container exits and the orchestrator can restart the lot.
exec /app/preuzmi serve
