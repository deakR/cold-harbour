#!/bin/sh
set -eu
LOG=/logs/app.log
touch "$LOG"
n=0
while true; do
  n=$((n + 1))
  echo "login attempt user1@example.com count=${n}" >>"$LOG"
  sleep 2
done
