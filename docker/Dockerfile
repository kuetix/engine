FROM golang:alpine as kuetix_backend

ARG USER_ID=0
ARG GROUP_ID=0
RUN echo "User is ${USER_ID}:${GROUP_ID}"

USER root

RUN if [ "$GROUP_ID" -ne 0 ]; then addgroup kuetix -g $GROUP_ID; else echo "Skipping group creation because GROUP_ID is 0"; fi
RUN if [ "$USER_ID" -ne 0 ]; then adduser kuetix -G kuetix -h /app -s /bin/false -S -D -u ${USER_ID}; else echo "Skipping user creation because USER_ID is 0"; fi

RUN echo "start"
RUN env
ENV TZ=Europe/Kiev
RUN date +%F-%T
RUN apk update
RUN apk add -v \
    net-tools \
    sudo \
    mc \
    iputils-ping \
    inetutils-telnet \
    wget \
    curl \
    procps \
    bind-tools \
    bash

USER "${USER_ID}:${GROUP_ID}"

RUN mkdir /app | true

WORKDIR /app

COPY docker/bin/* /opt/bin/

ENTRYPOINT ["bash", "/opt/bin/entrypoint.sh"]
