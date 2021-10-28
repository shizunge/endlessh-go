# Examples

## docker compose

This is an example how to setup endlessh-go, Prometheus, and Grafana using [docker compose](https://docs.docker.com/compose/). The reference of the compose file can be find [here](https://docs.docker.com/compose/compose-file/).

You also need `prometheus.yml` as a Prometheus configuration. [Here](https://prometheus.io/docs/prometheus/latest/configuration/configuration/) is more about Prometheus configuration.

Although Grafana data source can be setup manually, to ease the setup, we provision a data source `grafana-datasource.yml` for Grafana.

To start the stack, in the `examples` folder, run:

```
sudo docker-compose up -d
```

This example exposes the following ports.

* **2222**: The SSH port. You may test endlessh-go by run `ssh -p 2222 localhost`. Your SSH client should hang. You can view the log of endlessh-go by run `sudo docker logs endlessh`.
* **2112**: The Prometheus metrics exported by endlessh-go. You can open the your web browser and go to [http://localhost:2112/metrics](http://localhost:2112/metrics) to view the metrics.
* **9090**: Prometheus web interface. Go to [http://localhost:9090](http://localhost:9090) in your web browser for Prometheus. You can check whether the target of endlessh-go is up (Click Status, then Targets).
* **3000**: Grafana. Go to [http://localhost:3000](http://localhost:3000) for Grafana. Use username `examples` and password `examples` to login.

In this example, we do not [provision a dashboard](https://grafana.com/tutorials/provision-dashboards-and-data-sources/) for Grafana. You need to manually load the endlessh-go dashboard, by either importing it from the Grafana.com using ID [15156](https://grafana.com/grafana/dashboards/15156), or pasting the dashboard JSON text to the text area. See the [Grafana documentation](https://grafana.com/docs/grafana/latest/dashboards/export-import/). Then select *Prometheus* as the data source.
