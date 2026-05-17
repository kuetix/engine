#!/usr/bin/env bash

USER_ID=$1
GROUP_ID=$2

USER_RECORD=$(cat < /etc/passwd | grep "${USER_ID}")

echo "USER_ID: $USER_ID"
echo "GROUP_ID: $GROUP_ID"
echo "USER_RECORD: $USER_RECORD"

if [ "$USER_RECORD" == "" ]; then
  echo "Creating user..."
  groupadd --gid ${GROUP_ID} php
  useradd -r -d /opt/application -s /bin/bash --uid ${USER_ID} -g ${GROUP_ID} -G root,www-data,crontab php
  chmod -R g+w /run
  chmod +t /run
  echo "...created."
else
  echo "User found and is: $USER_RECORD"
fi
