FROM golang:1.20.3-alpine3.17 AS build

# environment settings
ARG GOOS="linux" \
    CGO_ENABLED="0"

# copy files to container
COPY . "/tmp/endlessh-go/"

# set build workdir
WORKDIR "/tmp/endlessh-go/"

# build app
RUN \
    go mod tidy \
    && go build -o "endlessh-go"


FROM alpine:3.17

# labels
LABEL org.opencontainers.image.title=endlessh-go
LABEL org.opencontainers.image.description="Endlessh: an SSH tarpit"
LABEL org.opencontainers.image.vendor="Shizun Ge"
LABEL org.opencontainers.image.licenses=GPLv3


# install packages
RUN \
    echo "**** installing base packages ****" \
    && apk update \
    && apk --no-cache add \
    ca-certificates \
    bash \
    procps \
    tzdata

# prepare container
RUN \
    echo "**** create default user ****" \
    && addgroup -S -g 2000 abc \
    && adduser -S -D -H -s /bin/bash -u 2000 -G abc abc

# cleanup installation
RUN \
    echo "**** cleanup ****" \
    && rm -rf \
    /tmp/* \
    /var/cache/apk/* \
    /var/tmp/*

# copy files to container
COPY --from=build "/tmp/endlessh-go/endlessh-go" "/usr/local/bin/"

# ssh / prometheus port
EXPOSE 2222 2112

USER abc

ENTRYPOINT  ["/usr/local/bin/endlessh-go"]

CMD ["-logtostderr", "-v=1"]
