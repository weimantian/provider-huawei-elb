# AGENTS.md

Repo-specific guidance for AI agents working in `ds-provider-huawei-elb`.

## Project status

**Greenfield — no code yet.** The authoritative spec is `OpenEverest ELB Provider 开发指南.md` (read it first). This repo will host a Go-based **OpenEverest v2 Provider plugin** that integrates Huawei Cloud ELB to give database instances external load-balanced access.

## Tech stack (non-negotiable)

- **Go 1.26+** (required by provider-sdk)
- **Huawei Cloud SDK**: `github.com/huaweicloud/huaweicloud-sdk-go-v3` — use **ELB v3** API, not v2
- **OpenEverest Provider SDK**: `github.com/openeverest/provider-sdk` (scaffolding + reconcile harness)
- Kubernetes via `client-go` / `controller-runtime`
- Deployed via **Helm** chart under `charts/provider-huawei-elb/`

## Initialization

Project is scaffolded with the SDK (do not hand-roll the layout):

```bash
provider-sdk init \
  --name provider-huawei-elb \
  --module github.com/your-org/provider-huawei-elb
```

The generated layout (cmd/, internal/provider/, definition/, charts/, generated/, examples/, Makefile, go.mod) is the contract — follow it. Full tree is in the dev guide §3.2.

## Commands

| Command | Purpose |
|---|---|
| `make generate` | Generate `generated/provider-spec.yaml` + `generated/rbac-rules.yaml` from Kubebuilder markers. **Run after editing RBAC or component types.** |
| `make run` | Run provider locally against a test Kubernetes cluster |
| `make helm-install` | Deploy via Helm to `everest-system` namespace |

Build/push images manually: `docker build -t <registry>/provider-huawei-elb:latest .`

Order matters: edit markers → `make generate` → `make run` to verify.

## Architecture invariants

- **Provider interface** (`internal/provider/provider.go`): implement all four — `Validate`, `Sync`, `Status`, `Cleanup`. `Sync` is the core reconcile entry: call Huawei ELB API, then create/update the Kubernetes `LoadBalancer` Service, then return endpoint info.
- **RBAC** (`internal/provider/rbac.go`): declared via **Kubebuilder markers** (`// +kubebuilder:rbac:...`), not hand-written YAML. The generated manifests are the source of truth.
- **`generated/` is read-only** — never edit by hand; regenerate with `make generate`.
- **Topologies** (`definition/topologies/*/topology.yaml`): `public-elb` and `internal-elb` are the two deployment modes. UI Schema lives inside topology YAML.
- **Provider registration**: via a `Provider` CR applied to the cluster; OpenEverest's controller auto-discovers it. CR `apiVersion: plugin.openeverest.io/v1alpha1`.

## Critical Huawei Cloud / Kubernetes details

- The Kubernetes `LoadBalancer` Service **must** carry the annotation `kubernetes.io/elb.id: <elbID>` to bind a pre-created ELB, **or** `kubernetes.io/elb.autocreate` with a JSON spec to let CCE auto-create one. Mixing both is undefined.
- ELB client auth: `basic.NewCredentialsBuilder()` with AK/SK + `projectId` + region.
- Namespace for created Services: `everest-system`.

## Commit convention

**DCO required.** Every commit must include `Signed-off-by: Your Name <email>` (use `git commit -s`). Commits without it will be rejected upstream.

## Constraints & gotchas

- **OpenEverest v2 is Developer Preview** (`v2.0.0-dev.1`). APIs may break before GA. v1 and v2 are incompatible — do not run both in one cluster.
- Provider SDK and OpenEverest core evolve fast; when behavior diverges from the dev guide, trust the **upstream repos** (not the guide).
- The dev guide's code samples are illustrative (e.g. hardcoded endpoint `"1.2.3.4:3306"`, placeholder `your-org`). Replace with real values; do not copy blindly.

## Reference implementations

When unsure about a pattern, study existing providers before inventing:

- **MongoDB (official)**: https://github.com/openeverest/plugin-mongodb-explorer
- **ClickHouse (community)**: https://github.com/scaledb-io/provider-altinity-clickhouse
- **Provider SDK docs**: https://github.com/openeverest/provider-sdk/blob/main/PROVIDER_DEVELOPMENT.md
- **OpenEverest main repo**: https://github.com/openeverest/openeverest

## Existing instruction files

- `CLAUDE.md` — general behavioral guidelines (think-before-coding, surgical changes, goal-driven). Applies to all agents.
- `OpenEverest ELB Provider 开发指南.md` — the full development spec. Read before any implementation work.
