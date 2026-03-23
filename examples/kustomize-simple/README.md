## kustomize

This is an example how to setup endlessh-go with existing Prometheus and Grafana using [kustomize](https://kustomize.io/).

This example assumes the cluster already has a Prometheus Operator based monitoring stack. It deploys:

- endlessh-go
- a Service exposing SSH and Prometheus metrics
- a `ServiceMonitor` for scraping endlessh-go metrics
- a Grafana dashboard `ConfigMap`

To deploy the stack, run:

```bash
kubectl apply -k examples/kustomize-simple
```

`dashboard.json` is added to a `ConfigMap` with label `grafana_dashboard=1`, which can be picked up by a Grafana sidecar based dashboard loader.

The `ServiceMonitor` in `monitor.yaml` scrapes the `metrics` port every `60s`. If your Prometheus stack only selects `ServiceMonitor` objects with specific labels, add the matching label in `kustomization.yaml`.

The `endlessh` Service exposes the following ports inside the cluster:

- **22**: SSH for endlessh-go
- **2112**: Prometheus metrics exported by endlessh-go
