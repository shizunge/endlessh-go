## docker compose

This is an example how to setup endlessh-go with the Maxmind GeoIP Database using [docker compose](https://docs.docker.com/compose/). The reference of the compose file can be found [here](https://docs.docker.com/compose/compose-file/).

To start the stack, in the _examples_ folder, run:

```
docker-compose up -d
```

The GeoIP Database will be saved in a mounted volume in: `./geo-data`. And the endlessh-go container will use this database to do the location lookups.

This example exposes the following ports. Except the SSH port, you should not expose other ports to public without protections (not included in this example) in production.

-   **2222**: The SSH port. You may test endlessh-go by running `ssh -p 2222 localhost`. Your SSH client should hang. View the log of endlessh-go by running `docker logs endlessh`.
-   **2112**: The Prometheus metrics exported by endlessh-go. Go to [http://localhost:2112/metrics](http://localhost:2112/metrics) in your web browser to view the metrics.
