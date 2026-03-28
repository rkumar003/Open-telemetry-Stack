# OpenTelemetry Observability Stack

Full-stack observability using OpenTelemetry — metrics and logs collected from a Go service, shipped through the OTel Collector, stored in Prometheus and Loki, and visualized in Grafana.

## Architecture

```
Go App (OTel SDK)
    │
    │  OTLP gRPC (metrics + logs)
    ▼
OTel Collector (contrib)
    │               │
    │ Prometheus     │ Loki push
    │ exporter       │ exporter
    ▼               ▼
Prometheus        Loki
    └──────┬───────┘
           ▼
        Grafana
        (pre-provisioned dashboards)
```

## Signals

| Signal  | SDK → Collector | Collector → Backend  |
|---------|-----------------|----------------------|
| Metrics | OTLP gRPC       | Prometheus (scrape)  |
| Logs    | OTLP gRPC       | Loki (push API)      |

## Metrics emitted by the app

| Metric                               | Type        | Description                          |
|--------------------------------------|-------------|--------------------------------------|
| `http.server.request.total`          | Counter     | Total HTTP requests (by route/code)  |
| `http.server.request.duration`       | Histogram   | Latency in ms (p50/p90/p99)          |
| `http.server.active_requests`        | UpDownGauge | In-flight requests                   |
| `http.server.errors.total`           | Counter     | 5xx error count                      |
| `system.cpu.utilization`             | Gauge       | Simulated CPU (0–1)                  |
| `system.memory.utilization`          | Gauge       | Simulated memory (0–1)               |

---

## Quick Start – Docker Compose

### Prerequisites
- Docker ≥ 24
- Docker Compose v2

### Run

```bash
cd otel-stack
docker compose up --build -d
```

### Ports

| Service         | URL                              |
|-----------------|----------------------------------|
| Grafana         | http://localhost:3000  (admin/admin) |
| Prometheus      | http://localhost:9090            |
| Loki            | http://localhost:3100            |
| OTel Collector  | grpc: 4317 / http: 4318          |
| Demo App        | http://localhost:8080            |
| Collector zPages| http://localhost:55679/debug/pipelinez |
| Collector Health| http://localhost:13133           |

### Grafana Dashboards (auto-provisioned)

- **OTel Demo – HTTP Metrics**: request rate, error rate, latency percentiles, CPU/memory gauges
- **OTel Demo – Logs Explorer**: log volume, error log stream, full log browser

### Stop

```bash
docker compose down -v   # -v removes volumes too
```

---

## Kubernetes – Helm

### Prerequisites
- kubectl configured against a cluster
- Helm ≥ 3.12
- A container registry to push the demo app image (or use a prebuilt one)

### 1. Build and push the app image

```bash
docker build -t your-registry/otel-demo-app:latest ./app
docker push your-registry/otel-demo-app:latest
```

### 2. Install the chart

```bash
helm upgrade --install otel-stack ./helm/otel-stack \
  --set global.clusterName=my-cluster \
  --set global.environment=staging \
  --set app.image.repository=your-registry/otel-demo-app \
  --set grafana.adminPassword=supersecret \
  --create-namespace
```

### 3. Access Grafana

```bash
kubectl port-forward -n monitoring svc/grafana 3000:3000
# Open http://localhost:3000
```

### 4. Enable Ingress (optional)

```bash
helm upgrade otel-stack ./helm/otel-stack \
  --reuse-values \
  --set grafana.ingress.enabled=true \
  --set grafana.ingress.host=grafana.your-domain.com
```

### Override values example (`my-values.yaml`)

```yaml
global:
  clusterName: prod-us-east
  environment: production

otelCollector:
  replicas: 3
  resources:
    limits:
      cpu: "2"
      memory: "1Gi"

prometheus:
  retention: "30d"
  storage:
    size: "100Gi"
    storageClass: gp3

grafana:
  adminPassword: "from-sealed-secret"
  ingress:
    enabled: true
    host: grafana.internal.example.com
```

```bash
helm upgrade --install otel-stack ./helm/otel-stack -f my-values.yaml
```

---

## Project Structure

```
otel-stack/
├── app/                        # Go service (OTel SDK instrumented)
│   ├── main.go                 # App logic + OTel setup
│   ├── go.mod
│   └── Dockerfile
├── otel-collector/
│   └── config.yaml             # Collector pipelines (metrics + logs)
├── prometheus/
│   └── prometheus.yml          # Scrape configs
├── loki/
│   └── loki-config.yaml        # Loki single-process config
├── grafana/
│   ├── provisioning/
│   │   ├── datasources/        # Auto-provision Prometheus + Loki
│   │   └── dashboards/         # Auto-load dashboard JSONs
│   └── dashboards/
│       ├── http-metrics.json   # HTTP metrics dashboard
│       └── logs-explorer.json  # Logs explorer dashboard
├── helm/
│   └── otel-stack/             # Helm chart (K8s deployment)
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
│           ├── _helpers.tpl
│           ├── namespace.yaml
│           ├── otel-collector.yaml
│           ├── prometheus.yaml
│           ├── loki.yaml
│           ├── grafana.yaml
│           └── app.yaml
└── docker-compose.yml
```

---

## Extending the Stack

### Add Traces
1. Add `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc` to `go.mod`
2. Init a `TracerProvider` in `main.go` using the same gRPC connection
3. Add a `traces` pipeline in the collector config with a Tempo or Jaeger exporter
4. Add Tempo as a Grafana datasource

### Add alerting
Drop a `prometheus-rules.yaml` ConfigMap referencing your alertmanager, or use Grafana's unified alerting (`GF_FEATURE_TOGGLES_ENABLE=ngalert`).

### Production hardening
- Replace the collector's in-memory queue with a persistent queue
- Use Loki in microservices mode with S3 object storage
- Mount Grafana dashboards from a ConfigMap (managed via GitOps)
- Use Sealed Secrets or Vault for `grafana.adminPassword`
