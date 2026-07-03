# provider-huawei-elb

[English](README.md) | 中文

一个 [OpenEverest](https://github.com/openeverest) v2 Provider 插件，集成**华为云 ELB**（弹性负载均衡），为数据库实例提供 VPC 内部或公网的负载均衡访问能力。

## 功能说明

当用户创建带 `provider: provider-huawei-elb` 的 OpenEverest `Instance` CR 时，本 Provider 会：

1. **创建华为云 ELB**（v3 API），部署到指定的 VPC/子网/可用区。
2. **创建 Kubernetes `LoadBalancer` Service**，携带 `kubernetes.io/elb.id: <elbID>` 注解，让 CCE 将预创建的 ELB 绑定到该 Service。
3. **返回连接信息**（主机 + 端口），在 ELB 达到 `ACTIVE` 状态后生效。
4. **清理资源**，在 Instance 删除时同时删除 ELB 和 Service。

CCE 会根据 Service spec 自动管理 ELB 的监听器、后端服务器组和健康检查——Provider 无需手动创建这些资源。

## 架构

### 组件

| 组件 | 类型 | 用途 |
|---|---|---|
| `elbEngine` | `elb-engine` | 网络参数：VPC ID、子网 ID、可用区 |
| `elbListener` | `elb-listener` | 监听器参数：协议、端口、后端端口（可选——省略时使用默认值） |

### 拓扑

| 拓扑 | 说明 |
|---|---|
| `public-elb` | 创建公网 ELB，带 EIP 和带宽 |
| `internal-elb` | 创建内网 ELB，仅 VPC 内可访问 |

### 调和流程

```
Instance CR 创建
       │
       ▼
   Validate ──── 检查 VPC/子网/可用区字段，公网 ELB 的带宽
       │
       ▼
     Sync ─────── 1. 从 Instance spec 解析配置
                   2. 加载华为云凭证（环境变量）
                   3. 检查已有 Service 的 ELB ID 注解
                   4. 若无 ELB ID，按名称查找 → 复用或创建
                   5. 创建/更新 K8s Service（带 elb.id 注解）
       │
       ▼
    Status ────── 1. 从 Service 注解获取 ELB ID
                   2. 通过华为云 API 查询 ELB 状态
                   3. 返回 Provisioning / Ready / Failed
       │
       ▼
   Cleanup ────── 1. 从 Service 获取 ELB ID（或按名称查找）
                   2. 通过华为云 API 删除 ELB
                   3. 删除 K8s Service
```

### 部署流程与角色分工

```
你（本地机器）          helm install          Kubernetes 集群
                         一条命令完成
    │                        │                      │
    │  docker build/push     │                      │
    │  (镜像已推送到 SWR)     │                      │
    │                        │                      │
    │  kubectl create secret │                      │
    │  (创建华为云凭证)       │                      │
    │                        │                      │
    │  helm install ──────────┼──→ 创建 Deployment ──→ K8s 自动拉镜像、启动容器
    │                        │    创建 Provider CR──→ OpenEverest 注册 Provider
    │                        │    创建 RBAC ────────→ 容器获得 K8s 权限
    │                        │                      │
    │  kubectl get pods ─────┼──────────────────────→ 查看容器是否 Running
    │  (验证部署)             │                      │
    │                        │                      │
    │  kubectl apply ────────┼──────────────────────→ 容器检测到 Instance CR
    │  (创建 Instance CR)     │                      │  → 调华为云 API 创建 ELB
    │                        │                      │  → 创建 K8s Service
```

| 角色 | 做什么 | 何时运行 |
|---|---|---|
| **本地机器** | `helm install` / `kubectl apply` / `kubectl get` | 部署和操作时 |
| **Provider 容器** | 监听 Instance CR → 调华为云 API → 创建/删除 ELB 和 Service | 7×24 持续运行 |
| **华为云 ELB** | 接收 API 调用，创建/删除 ELB 实例 | 被 Provider 调用时 |

> `helm install` 是唯一的“启动”步骤。之后容器在集群里持续运行，你只需用 `kubectl apply` 创建 Instance CR 来触发它工作。

## 前置条件

- **Go 1.26+**
- Kubernetes 集群（CCE 或本地 k3d/kind 开发环境）
- 已安装 [OpenEverest v2 CRD](https://github.com/openeverest/openeverest)
- 华为云凭证：
  - AK（访问密钥 ID）
  - SK（秘密访问密钥）
  - 区域（如 `cn-north-4`）
  - 项目 ID

## 快速开始

### 1. 配置凭证

创建 Kubernetes Secret 保存华为云凭证：

```bash
kubectl create secret generic huawei-cloud-credentials \
  --from-literal=ak=<YOUR_AK> \
  --from-literal=sk=<YOUR_SK> \
  --from-literal=project-id=<YOUR_PROJECT_ID> \
  -n everest-system
```

### 2. 部署 Provider

```bash
# 构建并推送镜像（或使用预构建镜像）
docker build -t <registry>/provider-huawei-elb:latest .

# 通过 Helm 安装
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

完整配置选项见 `charts/provider-huawei-elb/values.yaml`。

### 3. 创建 Instance

```bash
# 最小配置 — 公网 ELB，使用默认监听器（TCP:3306）
kubectl apply -f examples/instance-simple.yaml

# 完整配置 — 公网 ELB，显式指定监听器参数
kubectl apply -f examples/instance-example.yaml

# 内网 — VPC 内部 ELB（无公网 IP）
kubectl apply -f examples/instance-internal-elb.yaml
```

**请将** `vpc-xxxxxxxx`、`subnet-xxxxxxxx` 和可用区值替换为你的实际华为云资源 ID。

### 4. 查看状态

```bash
kubectl get instance <name> -o yaml
# Status.connectionDetails.host 和 .port 在 Ready 时显示 ELB 端点
```

## 配置说明

### 环境变量

| 变量 | 必需 | 说明 |
|---|---|---|
| `HUAWEI_CLOUD_AK` | 是 | 华为云访问密钥 ID |
| `HUAWEI_CLOUD_SK` | 是 | 华为云秘密访问密钥 |
| `HUAWEI_CLOUD_REGION` | 是 | 区域（如 `cn-north-4`、`cn-east-3`） |
| `HUAWEI_CLOUD_PROJECT_ID` | 是 | 该区域的项目 ID |

### Instance CR 字段

```yaml
apiVersion: core.openeverest.io/v1alpha1
kind: Instance
metadata:
  name: my-elb
spec:
  provider: provider-huawei-elb
  topology:
    type: public-elb          # 或 "internal-elb"
    config:                    # 仅 public-elb 需要
      bandwidthSize: 20        # Mbit/s（1-2000，默认 10）
      bandwidthChargeMode: traffic  # "traffic" 或 "bandwidth"
      publicIpNetworkType: 5_bgp    # 默认 "5_bgp"
  components:
    elbEngine:
      type: elb-engine
      customSpec:
        vpcId: vpc-xxxxxxxx
        vipSubnetCidrId: subnet-xxxxxxxx
        availabilityZoneList:
          - cn-north-4a
          - cn-north-4b
    elbListener:               # 可选 — 默认: TCP:3306→3306
      type: elb-listener
      customSpec:
        protocol: TCP           # TCP、HTTP 或 HTTPS
        port: 3306              # 前端端口
        backendPort: 3306       # 后端端口
```

### 默认值

省略 `elbListener` 时，Provider 使用以下默认值：
- 协议：`TCP`
- 端口：`3306`
- 后端端口：`3306`

## 开发

### 项目结构

```
cmd/provider/              # 程序入口
internal/
  provider/
    provider.go            # ProviderInterface: Validate/Sync/Status/Cleanup
    config.go              # 从 Instance spec 解析配置
    service.go             # K8s Service 管理（创建/获取/删除）
    rbac.go                # Kubebuilder RBAC 标记
  huaweicloud/
    client.go              # ELB v3 客户端构造
    elb.go                 # ELB CRUD 操作（创建/查询/查找/删除）
  common/
    spec.go                # 共享常量（名称、注解、默认值）
definition/
  provider.yaml            # Provider 名称 + 组件映射
  versions.yaml            # 组件类型版本目录
  components/types.go      # 组件自定义规格 Go 类型
  topologies/
    public-elb/            # topology.yaml + types.go
    internal-elb/          # topology.yaml + types.go
config/rbac/role.yaml      # 生成的 ClusterRole（勿手动编辑）
charts/provider-huawei-elb/ # Helm Chart
  generated/               # 生成的 RBAC + provider spec（勿手动编辑）
  templates/               # Helm 模板
examples/                  # 示例 Instance CR
```

### Make 命令

| 命令 | 说明 |
|---|---|
| `make generate` | 从标记生成 RBAC、Helm 同步和 provider spec |
| `make run` | 本地运行 Provider，连接 Kubernetes 集群 |
| `make build` | 构建 Provider 二进制 |
| `make docker-build` | 构建容器镜像 |
| `make helm-install` | 通过 Helm 部署 |
| `make helm-template` | 渲染 Helm 模板（试运行） |
| `make test` | 运行单元测试 |
| `make lint` | 运行 golangci-lint |
| `make verify` | 检查生成文件是否最新（CI 用） |
| `make k3d-cluster-up` | 创建本地 k3d 集群 |
| `make k3d-cluster-down` | 删除本地 k3d 集群 |

### 本地开发

```bash
# 创建本地 k3d 集群
make k3d-cluster-up

# 本地运行 Provider
export HUAWEI_CLOUD_AK=<ak>
export HUAWEI_CLOUD_SK=<sk>
export HUAWEI_CLOUD_REGION=cn-north-4
export HUAWEI_CLOUD_PROJECT_ID=<project-id>
make run

# 销毁集群
make k3d-cluster-down
```

### 代码生成

RBAC 权限通过 [Kubebuilder 标记](https://book.kubebuilder.io/reference/markers/rbac) 在 `internal/provider/rbac.go` 中声明，而非手写 YAML。编辑标记或组件类型后执行：

```bash
make generate
```

`charts/` 下的 `generated/` 目录是只读的——始终重新生成，不要手动编辑。

## 参考实现

- [MongoDB Provider（官方）](https://github.com/openeverest/plugin-mongodb-explorer)
- [ClickHouse Provider（社区）](https://github.com/scaledb-io/provider-altinity-clickhouse)
- [Provider SDK 文档](https://github.com/openeverest/provider-sdk/blob/main/PROVIDER_DEVELOPMENT.md)

## 许可证

Apache License 2.0 — 详见 [LICENSE](LICENSE)。
