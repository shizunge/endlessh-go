# Examples

> The default container user has uid 65534.

## docker-simple

An example how to setup endlessh-go, Prometheus, and Grafana using [docker compose](https://docs.docker.com/compose/).

## docker-maxmind

An example how to setup endlessh-go with the Maxmind GeoIP Database.

### Using privileged ports (<1024) on docker

If you want to run the image with privileged ports (below 1025), you need to set the container user to root:

```yml
user: root
```
