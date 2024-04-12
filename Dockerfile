FROM golang AS build

RUN mkdir /endlessh
ADD . /endlessh
WORKDIR /endlessh
RUN go mod tidy
RUN CGO_ENABLED=0 go build -o endlessh .
RUN CGO_ENABLED=0 go build -o cache cache.go
RUN CGO_ENABLED=0 go build -o report report.go

FROM gcr.io/distroless/base

LABEL org.opencontainers.image.title=endlessh-go
LABEL org.opencontainers.image.description="Endlessh: an SSH tarpit"
LABEL org.opencontainers.image.vendor="Shizun Ge"
LABEL org.opencontainers.image.licenses=GPLv3

COPY --from=build /endlessh/endlessh /endlessh
COPY --from=build /endlessh/cache /app/cache
COPY --from=build /endlessh/report /app/report
COPY --from=build /endlessh/reportedIps.txt /app/reportedIps.txt

EXPOSE 2222 2112
USER nobody

# Run endlessh in the background
ENTRYPOINT ["/endlessh"]
CMD ["-logtostderr", "-v=1"]

# Run the reporting script in the background
CMD ["/app/report"]
