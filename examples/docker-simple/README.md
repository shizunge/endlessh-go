## docker compose

This is an example how to setup endlessh-go, Prometheus, and Grafana using [docker compose](https://docs.docker.com/compose/). The reference of the compose file can be found [here](https://docs.docker.com/compose/compose-file/).

*prometheus.yml* is used as a [Prometheus configuration](https://prometheus.io/docs/prometheus/latest/configuration/configuration/).

*grafana-datasource.yml* is used to provision a data source for Grafana to ease the setup, though Grafana data source can also be setup manually.

To start the stack, in the *examples* folder, run:

```
docker-compose up -d
```

This example exposes the following ports. Except the SSH port, you should not expose other ports to public without protections (not included in this example) in production.

* **2222**: The SSH port. You may test endlessh-go by running `ssh -p 2222 localhost`. Your SSH client should hang. View the log of endlessh-go by running `docker logs endlessh`.
* **2112**: The Prometheus metrics exported by endlessh-go. Go to [http://localhost:2112/metrics](http://localhost:2112/metrics) in your web browser to view the metrics.
* **9090**: Prometheus web interface. Go to [http://localhost:9090](http://localhost:9090) in your web browser for Prometheus. You can check whether the target of endlessh-go is up (Click Status, then Targets).
* **3000**: Grafana. Go to [http://localhost:3000](http://localhost:3000) in your web browser for Grafana. Use username *examples* and password *examples* to login.

In this example, we do not provision a dashboard for Grafana. You need to manually load the endlessh-go dashboard, by either importing it from the Grafana.com using ID [15156](https://grafana.com/grafana/dashboards/15156), or pasting the dashboard JSON text to the text area. See the [Grafana documentation](https://grafana.com/docs/grafana/latest/dashboards/export-import/) about import. Then select *Prometheus* as the data source.
