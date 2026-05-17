#!/bin/sh

echo "Welcome to backend"
while [ true ]; do
  cd /app/ || exit 1
  pwd -L
  echo "Building, env variables"
  env
  go env
  echo "Building to backend"
  make build
  echo "Run backend"
  cd /app/runtime/ || exit 1
  pwd -L
  /app/runtime/kuetix
  sleep 3
done
