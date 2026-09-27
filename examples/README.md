# Examples

> The default container user has uid 65534.

## [toml-config](./config.toml)

An example toml configuration.

## [docker-simple](./docker-simple)

An example how to setup endlessh-go, Prometheus, and Grafana using [docker compose](https://docs.docker.com/compose/).

## [docker-maxmind](./docker-maxmind)

An example how to setup endlessh-go with the Maxmind GeoIP Database.

## [kustomize-simple](./kustomize-simple)

An example how to setup endlessh-go using [kustomize](https://kustomize.io/).

## FAQ

### Bind to privileged ports (<1024) in a container

You need to add capability `NET_BIND_SERVICE` to the program.

If you are using docker, this can be done via cli argument [`--cap-add`](https://docs.docker.com/engine/reference/run/#runtime-privilege-and-linux-capabilities) or [`cap_add`](https://docs.docker.com/compose/compose-file/compose-file-v3/#cap_add-cap_drop) in the docker compose file.
