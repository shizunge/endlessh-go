FROM golang:alpine AS build

ADD . /go/src/app
WORKDIR /go/src/app
RUN go mod tidy
RUN go build -o endlessh .

FROM alpine:latest

LABEL org.opencontainers.image.title=endlessh-go
LABEL org.opencontainers.image.description="Endlessh: an SSH tarpit"
LABEL org.opencontainers.image.vendor="Shizun Ge"
LABEL org.opencontainers.image.licenses=GPLv3

COPY --from=build /go/src/app/endlessh /usr/bin/endlessh
EXPOSE 2222 2112
USER nobody
ENTRYPOINT  ["/usr/bin/endlessh"]
CMD ["-logtostderr", "-v=1"]
