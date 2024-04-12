FROM golang AS build

RUN mkdir /endlessh
ADD . /endlessh
WORKDIR /endlessh
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o endlessh .

FROM gcr.io/distroless/base

LABEL org.opencontainers.image.title=endlessh-go
LABEL org.opencontainers.image.description="Endlessh: an SSH tarpit"
LABEL org.opencontainers.image.vendor="Shizun Ge"
LABEL org.opencontainers.image.licenses=GPLv3

COPY --from=build /endlessh/endlessh /endlessh
EXPOSE 2222 2112
USER nobody
ENTRYPOINT  ["/endlessh"]
CMD ["-logtostderr", "-v=1"]
