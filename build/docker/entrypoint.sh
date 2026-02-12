#!/bin/sh
# Ensure /etc/machine-id exists for machineid library (device identification).
# Generated once per container instance so each deployment gets a unique device ID.
# To persist across container restarts, mount a volume or bind-mount at /etc/machine-id.
if [ ! -s /etc/machine-id ]; then
  od -x /dev/urandom | head -1 | awk '{OFS=""; print $2,$3,$4,$5,$6,$7,$8,$9}' > /etc/machine-id
fi

exec /server "$@"
