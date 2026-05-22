#!/bin/sh
set -e

if [ "$(id -u)" = "0" ]; then
    chown -R shellport:shellport /config
    exec gosu shellport:shellport /shellport "$@"
fi

exec /shellport "$@"
