# endlessh-go

A golang implementation of [endlessh](https://nullprogram.com/blog/2019/03/22/) exporting Prometheus metrics, visualized by a Grafana dashboard.

![screenshot](https://github.com/shizunge/endlessh-go/raw/main/dashboard/screenshot.png)

## Introduction

[Endlessh](https://nullprogram.com/blog/2019/03/22/) is a great idea that not only blocks the brute force SSH attacks, but also wastes attackers time as a kind of counter-attack. Besides trapping the attackers, I also want to visualize the Geolocations and other statistics of the sources of attacks. Unfortunately the wonderful original [C implementation of endlessh](https://github.com/skeeto/endlessh) only provides text based log, but I do not like the solution that writes extra scripts to parse the log outputs, then exports the results to a dashboard, because it would introduce extra layers in my current setup and it would depend on the format of the text log file rather than some structured data. Thus I create this golang implementation of endlessh to export [Prometheus](https://prometheus.io/) metrics and a [Grafana](https://grafana.com/) dashboard to visualize them.

If you want a dashboard of sources of attacks and do not mind the endlessh server, besides trapping the attackers, does extra things including: translating IP to Geohash, exporting Prometheus metrics, and using more memory (about 10MB), this is the solution for you.

## Getting Started

Clone the repo then build from source:

```
go build .
./endlessh-go &
```

Alternatively, you can use the [docker image](https://hub.docker.com/r/shizunge/endlessh-go):

```
docker run -d -p 2222:2222 shizunge/endlessh-go -logtostderr -v=1
```

It listens to port `2222` by default.

Then you can try to connect to the endlessh server. Your SSH client should hang there.

```
ssh -p 2222 localhost
```

If you want log like the [C implementation](https://github.com/skeeto/endlessh), you need to set both CLI arguments `-logtostderr` and `-v=1`, then the log will go to stderr. You can set different log destinations via CLI arguments.

Also check out [examples](./examples/README.md) for the setup of the full stack.

## Usage

`./endlessh-go --help`

```
Usage of ./endlessh-go
  -alsologtostderr
        log to standard error as well as files
  -conn_type string
        Connection type. Possible values are tcp, tcp4, tcp6 (default "tcp")
  -enable_prometheus
        Enable prometheus
  -geoip_supplier string
        Supplier to obtain Geohash of IPs. Possible values are "off", "ip-api", "max-mind-db" (default "off")
  -host string
        SSH listening address (default "0.0.0.0")
  -interval_ms int
        Message millisecond delay (default 1000)
  -line_length int
        Maximum banner line length (default 32)
  -log_backtrace_at value
        when logging hits line file:N, emit a stack trace
  -log_dir string
        If non-empty, write log files in this directory
  -log_link string
        If non-empty, add symbolic links in this directory to the log files
  -logbuflevel int
        Buffer log messages logged at this level or lower (-1 means don't buffer; 0 means buffer INFO only; ...). Has limited applicability on non-prod platforms.
  -logtostderr
        log to standard error instead of files
  -max_clients int
        Maximum number of clients (default 4096)
  -max_mind_db string
        Path to the MaxMind DB file.
  -port value
        SSH listening port. You may provide multiple -port flags to listen to multiple ports. (default "2222")
  -prometheus_clean_unseen_seconds int
        Remove series if the IP is not seen for the given time. Set to 0 to disable. (default 0)
  -prometheus_entry string
        Entry point for prometheus (default "metrics")
  -prometheus_host string
        The address for prometheus (default "0.0.0.0")
  -prometheus_port string
        The port for prometheus (default "2112")
  -stderrthreshold value
        logs at or above this threshold go to stderr (default 2)
  -v value
        log level for V logs
  -vmodule value
        comma-separated list of pattern=N settings for file-filtered logging
```

## Metrics

Endlessh-go exports the following Prometheus metrics.

| Metric                               | Type  | Description  |
|--------------------------------------|-------|--------------|
| endlessh_client_open_count_total     | count | Total number of clients that tried to connect to this host. |
| endlessh_client_closed_count_total   | count | Total number of clients that stopped connecting to this host. |
| endlessh_sent_bytes_total            | count | Total bytes sent to clients that tried to connect to this host. |
| endlessh_trapped_time_seconds_total  | count | Total seconds clients spent on endlessh. |
| endlessh_client_open_count           | count | Number of connections of clients. <br> Labels: <br> <ul><li> `ip`: Remote IP of the client </li> <li> `local_port`: Local port the program listens to </li> <li> `country`: Country of the IP </li> <li> `location`: Country, Region, and City </li> <li> `geohash`: Geohash of the location </li></ul> |
| endlessh_client_trapped_time_seconds | count | Seconds a client spends on endlessh. <br> Labels: <br> <ul><li> `ip`: Remote IP of the client </li> <li> `local_port`: Local port the program listens to </li></ul> |

The metrics is off by default, you can turn it via the CLI argument `-enable_prometheus`.

It listens to port `2112` and entry point is `/metrics` by default. The port and entry point can be changed via CLI arguments.

The endlessh-go server stores the geohash of attackers as a label on `endlessh_client_open_count`, which is also off by default. You can turn it on via the CLI argument `-geoip_supplier`. The endlessh-go uses service from [ip-api](https://ip-api.com/), which may enforce a query rate and limit commercial use. Visit their website for their terms and policies.

You could also use an offline GeoIP database from [MaxMind](https://www.maxmind.com) by setting `-geoip_supplier` to _max-mind-db_ and `-max_mind_db` to the path of the database file.

## Dashboard

The dashboard requires Grafana 8.2.

You can import the dashboard from Grafana.com using ID [15156](https://grafana.com/grafana/dashboards/15156)

The dashboard visualizes data for the selected time range.

The IP addresses are clickable and link you to the [ARIN](https://www.arin.net/) database.

## Contacts

If you have any problems or questions, please contact me through a [GitHub issue](https://github.com/shizunge/endlessh-go/issues)
