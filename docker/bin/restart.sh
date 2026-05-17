#!/bin/bash

cd /opt/application/ || exit 1
pid=$(cat /var/run/application.pid)
echo "Restarting, found pid: $pid"
kill -SIGQUIT "$pid"
