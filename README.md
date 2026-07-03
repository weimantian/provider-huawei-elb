# provider-huawei-elb

[English](README.md) | [中文](README.zh-CN.md)

An [OpenEverest](https://github.com/openeverest) v2 provider that integrates **Huawei Cloud ELB** (Elastic Load Balance) to give database instances load-balanced access from inside or outside the VPC.

## What It Does

When an OpenEverest `Instance` CR is created with `provider: provider-huawei-elb`, this provider:

1. **Creates a Huawei Cloud ELB** (v3 API) in the specified VPC/subnet/availability zones.
2. **Creates a Kubernetes `LoadBalancer` Service** annotated with `kubernetes.io/elb.id: <elbID>`, which tells CCE to bind the pre-created ELB to the Service.
3. **Returns connection details** (host + port) once the ELB reaches `ACTIVE` status.
4. **Cleans up** both the ELB and the Service when the Instance is deleted.

CCE automatically manages the ELB listener, backend pool, and health check based on the Service spec — the provider does not need to create those manually.

## Architecture

### Components

| Component | Type | Purpose |
|---|---|---|
| `elbEngine` | `elb-engine` | Network parameters: VPC ID, subnet ID, availability zones |
| `elbListener` | `elb-listener` | Listener parameters: protocol, port, backend port (optional — uses defaults if omitted) |

### Topologies

| Topology | Description |
|---|---|
| `public-elb` | Creates a public-facing ELB with an EIP and bandwidth |
| `internal-elb` | Creates an internal ELB accessible only within the VPC |

### Reconcile Flow

```
Instance CR created
       |
       v
   Validate ──── checks VPC/subnet/AZ fields, bandwidth for public-elb
       |
       v
     Sync ─────── 1. Resolve config from Instance spec
                   2. Load Huawei Cloud credentials (env vars)
                   3. Check existing Service for ELB ID annotation
                   4. If no ELB ID, search by name -> reuse or create
                   5. Create/Update K8s Service with elb.id annotation
       |
       v
    Status ────── 1. Get ELB ID from Service annotation
                   2. Query ELB provisioning status via Huawei Cloud API
                   3. Return Provisioning / Ready / Failed
       |
       v
   Cleanup ────── 1. Get ELB ID from Service (or find by name)
                   2. Delete the ELB via Huawei Cloud API
                   3. Delete the K8s Service
```

## Prerequisites

- **Go 1.26+**
- A Kubernetes cluster (CCE or local k3d/kind for development)
- [OpenEverest v2 CRDs](https://github.com/openeverest/openeverest) installed
- Huawei Cloud credentials:
  - AK (Access Key ID)
  - SK (Secret Access Key)
  - Region (e.g. `cn-north-4`)
  - Project ID

## Quick Start

### 1. Configure Credentials

Create a Kubernetes Secret with your Huawei Cloud credentials:

```bash
kubectl create secret generic huawei-cloud-credentials \
  --from-literal=ak=<YOUR_AK> \
  --from-literal=sk=<YOUR_SK> \
  --from-literal=project-id=<YOUR_PROJECT_ID> \
  -n everest-system
```

### 2. Deploy the Provider

```bash
# Build and push the image (or use a pre-built one)
docker build -t <registry>/provider-huawei-elb:latest .

# Install via Helm
helm install provider-huawei-elb charts/provider-huawei-elb \
  --create-namespace \
  --namespace everest-system \
  --set image.repository=<registry>/provider-huawei-elb \
  --set image.tag=latest \
  --set "extraEnv[0].name=HUAWEI_CLOUD_AK" \
  --set "extraEnv[0].valueFrom.secretKeyRef.name=huawei-cloud-credentials" \
  --set "extraEnv[0].valueFrom.secretKeyRef.key=ak" \
  --set "extraEnv[1].name=HUAWEI_CLOUD_SK" \
  --set "extraEnv[1].valueFrom.secretKeyRef.name=huawei-cloud-credentials" \
  --set "extraEnv[1].valueFrom.secretKeyRef.key=sk" \
  --set "extraEnv[2].name=HUAWEI_CLOUD_REGION" \
  --set "extraEnv[2].value=cn-north-4" \
  --set "extraEnv[3].name=HUAWEI_CLOUD_PROJECT_ID" \
  --set "extraEnv[3].valueFrom.secretKeyRef.name=huawei-cloud-credentials" \
  --set "extraEnv[3].valueFrom.secretKeyRef.key=project-id"
```

See `charts/provider-huawei-elb/values.yaml` for all configuration options.

### 3. Create an Instance

```bash
# Minimal — public ELB with default listener (TCP:3306)
kubectl apply -f examples/instance-simple.yaml

# Full — public ELB with explicit listener config
kubectl apply -f examples/instance-example.yaml

# Internal — VPC-internal ELB (no public IP)
kubectl apply -f examples/instance-internal-elb.yaml
```

**Remember to replace** `vpc-xxxxxxxx`, `subnet-xxxxxxxx`, and availability zone values with your actual Huawei Cloud resource IDs.

### 4. Check Status

```bash
kubectl get instance <name> -o yaml
# Status.connectionDetails.host and .port show the ELB endpoint when Ready
```

## Configuration

### Environment Variables

| Variable | Required | Description |
|---|---|---|
| `HUAWEI_CLOUD_AK` | Yes | Huawei Cloud Access Key ID |
| `HUAWEI_CLOUD_SK` | Yes | Huawei Cloud Secret Access Key |
| `HUAWEI_CLOUD_REGION` | Yes | Region (e.g. `cn-north-4`, `cn-east-3`) |
| `HUAWEI_CLOUD_PROJECT_ID` | Yes | Project ID for the region |

### Instance CR Fields

```yaml
apiVersion: core.openeverest.io/v1alpha1
kind: Instance
metadata:
  name: my-elb
spec:
  provider: provider-huawei-elb
  topology:
    type: public-elb          # or "internal-elb"
    config:                    # only for public-elb
      bandwidthSize: 20        # Mbit/s (1-2000, default 10)
      bandwidthChargeMode: traffic  # "traffic" or "bandwidth"
      publicIpNetworkType: 5_bgp    # default "5_bgp"
  components:
    elbEngine:
      type: elb-engine
      customSpec:
        vpcId: vpc-xxxxxxxx
        vipSubnetCidrId: subnet-xxxxxxxx
        availabilityZoneList:
          - cn-north-4a
          - cn-north-4b
    elbListener:               # optional — defaults: TCP:3306->3306
      type: elb-listener
      customSpec:
        protocol: TCP           # TCP, HTTP, or HTTPS
        port: 3306              # front-end port
        backendPort: 3306       # back-end port
```

### Defaults

When `elbListener` is omitted, the provider uses:
- Protocol: `TCP`
- Port: `3306`
- BackendPort: `3306`

## Development

### Project Structure

```
cmd/provider/              # Entry point
internal/
  provider/
    provider.go            # ProviderInterface: Validate/Sync/Status/Cleanup
    config.go              # ResolveConfig from Instance spec
    service.go             # K8s Service management (create/get/delete)
    rbac.go                # Kubebuilder RBAC markers
  huaweicloud/
    client.go              # ELB v3 client construction
    elb.go                 # ELB CRUD operations (Create/Show/Find/Delete)
  common/
    spec.go                # Shared constants (names, annotations, defaults)
definition/
  provider.yaml            # Provider name + component mapping
  versions.yaml            # Component type version catalog
  components/types.go      # Component custom spec Go types
  topologies/
    public-elb/            # topology.yaml + types.go
    internal-elb/          # topology.yaml + types.go
config/rbac/role.yaml      # Generated ClusterRole (do not edit)
charts/provider-huawei-elb/ # Helm chart
  generated/               # Generated RBAC + provider spec (do not edit)
  templates/               # Helm templates
examples/                  # Example Instance CRs
```

### Make Targets

| Target | Description |
|---|---|
| `make generate` | Generate RBAC, Helm sync, and provider spec from markers |
| `make run` | Run the provider locally against a Kubernetes cluster |
| `make build` | Build the provider binary |
| `make docker-build` | Build the container image |
| `make helm-install` | Deploy via Helm |
| `make helm-template` | Render Helm templates (dry-run) |
| `make test` | Run unit tests |
| `make lint` | Run golangci-lint |
| `make verify` | Check generated files are up-to-date (CI) |
| `make k3d-cluster-up` | Create a local k3d cluster |
| `make k3d-cluster-down` | Delete the local k3d cluster |

### Local Development

```bash
# Create a local k3d cluster
make k3d-cluster-up

# Run the provider locally
export HUAWEI_CLOUD_AK=<ak>
export HUAWEI_CLOUD_SK=<sk>
export HUAWEI_CLOUD_REGION=cn-north-4
export HUAWEI_CLOUD_PROJECT_ID=<project-id>
make run

# Tear down
make k3d-cluster-down
```

### Code Generation

RBAC permissions are declared via [Kubebuilder markers](https://book.kubebuilder.io/reference/markers/rbac) in `internal/provider/rbac.go`, not hand-written YAML. After editing markers or component types:

```bash
make generate
```

The `generated/` directories under `charts/` are read-only — always regenerate, never hand-edit.

## Reference Implementations

- [MongoDB Provider (official)](https://github.com/openeverest/plugin-mongodb-explorer)
- [ClickHouse Provider (community)](https://github.com/scaledb-io/provider-altinity-clickhouse)
- [Provider SDK docs](https://github.com/openeverest/provider-sdk/blob/main/PROVIDER_DEVELOPMENT.md)

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
