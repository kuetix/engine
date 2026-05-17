#!/usr/bin/env bash

res=$(curl -sSf http://127.0.0.1:9000 2>&1 | awk '{ print ($2 == "(56)" ? 0 : 1) }')
echo "exit $res"
exit $res
