# OpenEverest ELB Provider 开发指南

本文档为 Coding Agent 提供开发 OpenEverest ELB Provider 插件的完整指南。目标是为华为云弹性负载均衡（ELB）服务开发一个独立的 Provider 插件，使 OpenEverest 能够通过华为云 ELB 为数据库实例提供外部访问能力。

> **重要说明**：本指南中的代码示例已根据 `provider-sdk v0.1.0` 和 `openeverest/v2 v2.0.0-dev.1` 的真实 API 进行了验证。OpenEverest v2 处于 Developer Preview 阶段，API 可能变化——当本指南与上游仓库行为不一致时，**以上游仓库源码为准**。


## 一、背景与架构概述

### 1.1 什么是 Provider

OpenEverest v2.0 从单体架构转向了模块化架构。**Provider** 是一个自包含的插件，封装了特定技术的所有逻辑——包括组件、拓扑、调和逻辑以及用于渲染创建和编辑表单的 UI Schema。

在旧架构下，添加一种新的数据库技术可能需要几周甚至几个月。而在 v2 架构中，集成新技术不再需要触及 OpenEverest 核心，任何开发者使用官方 SDK 即可在几天内完成。

Provider 模型的关键优势：
- **独立发布周期**：更新 Provider 无需发布新版本的 OpenEverest server 或 operator
- **快速特性同步**：上游 operator 增加特性时，只需更新 Provider 插件
- **多拓扑支持**：单个 Provider 可提供多种部署架构

### 1.2 ELB Provider 的定位

ELB Provider 属于**网络类 Provider**，其职责是：
1. 接收 OpenEverest 核心发来的负载均衡器创建请求
2. 调用华为云 ELB API 创建 ELB 实例
3. 在 Kubernetes 集群中创建 `LoadBalancer` 类型的 Service，并通过 `kubernetes.io/elb.id` 注解绑定预创建的 ELB
4. 将 ELB 信息（如公网 IP）同步回 OpenEverest

### 1.3 技术栈

| 组件 | 技术 |
|------|------|
| 开发语言 | Go 1.26+ |
| Kubernetes | 通过 client-go 与 API Server 交互 |
| 华为云 SDK | `huaweicloud-sdk-go-v3`（ELB v3 API） |
| OpenEverest SDK | `github.com/openeverest/provider-sdk`（脚手架 + 生成工具） |
| Provider 运行时 | `github.com/openeverest/openeverest/v2/provider-runtime`（控制器框架） |
| 包管理 | Go Modules |
| 部署工具 | Helm |


## 二、环境准备

### 2.1 前置条件

- Go 1.26+ 已安装并配置好 `GOPATH`
- Kubernetes 集群（本地可用 k3d 或 kind）
- OpenEverest 已安装（参考 Quickstart Guide）
- 华为云账号及 ELB 服务的访问凭证（AK/SK）

### 2.2 安装 Provider SDK

Provider SDK 是开发的基石，提供脚手架和护栏，开发者无需从零编写复杂的 Kubernetes 调和逻辑：

```bash
go install github.com/openeverest/provider-sdk@latest
```

或直接在项目中使用 `go run` 引用。


## 三、项目创建与初始化

### 3.1 使用 SDK 初始化项目

```bash
provider-sdk init \
  --name provider-huawei-elb \
  --module github.com/openeverest/provider-huawei-elb
```

> **注意**：`--module` 必须使用 `github.com/openeverest/provider-huawei-elb`，与 go.mod 中的 module 声明一致。

### 3.2 项目结构

初始化后生成的标准结构如下（加 `★` 标记的为开发者需要编辑的文件）：

```
provider-huawei-elb/
├── cmd/
│   └── provider/
│       └── main.go                          # 程序入口
├── internal/
│   ├── common/
│   │   └── spec.go                        ★ # 组件名称、拓扑名称等常量
│   ├── huaweicloud/
│   │   ├── client.go                      ★ # 华为云 ELB 客户端构造
│   │   └── elb.go                         ★ # ELB CRUD 操作封装
│   └── provider/
│       ├── config.go                      ★ # 从 Instance spec 解析 ELBConfig
│       ├── service.go                     ★ # K8s LoadBalancer Service 管理
│       ├── provider.go                    ★ # ProviderInterface 实现（Validate/Sync/Status/Cleanup）
│       └── rbac.go                        ★ # Kubebuilder RBAC 标记
├── definition/
│   ├── provider.yaml                      ★ # Provider 名称 + 组件→类型映射
│   ├── versions.yaml                      ★ # 组件类型版本/镜像目录
│   ├── types.go                             # 共享 Go 类型（通常为空）
│   ├── components/
│   │   └── types.go                       ★ # 组件自定义规格类型（CustomSpec structs）
│   └── topologies/
│       ├── public-elb/
│       │   ├── topology.yaml              ★ # 公网 ELB 拓扑配置 + UI Schema
│       │   └── types.go                   ★ # 拓扑配置 Go 类型
│       └── internal-elb/
│           ├── topology.yaml              ★ # 私网 ELB 拓扑配置 + UI Schema
│           └── types.go                   ★ # 拓扑配置 Go 类型
├── config/
│   └── rbac/
│       └── role.yaml                        # 生成的 ClusterRole（勿手动编辑）
├── charts/
│   └── provider-huawei-elb/                 # Helm Chart（部署用）
│       ├── generated/                       # 自动生成（勿手动编辑）
│       │   ├── rbac-rules.yaml
│       │   └── provider-spec.yaml
│       ├── templates/
│       └── values.yaml
├── examples/
│   ├── instance-simple.yaml               ★ # 最小示例
│   ├── instance-example.yaml              ★ # 完整示例
│   └── instance-internal-elb.yaml         ★ # 内网 ELB 示例
├── dev/
│   └── k3d_config.yaml                      # 本地 k3d 集群配置
├── gen.go                                    # go:generate 入口
├── Makefile
├── Dockerfile
├── go.mod
└── go.sum
```

> **与早期文档的差异**：
> - `internal/common/` 不在 `internal/provider/` 下，而是独立的包
> - `generated/` 目录在 `charts/provider-huawei-elb/generated/` 下，不在项目根目录
> - 每个拓扑目录同时需要 `topology.yaml` 和 `types.go`
> - `config/rbac/role.yaml` 由 `make manifests` 从 RBAC 标记生成


## 四、定义 Provider 蓝图

### 4.1 定义组件（Components）

组件通过 **YAML 声明** + **Go 类型** 两步定义。

**第一步**：在 `definition/provider.yaml` 中声明组件名称和类型映射：

```yaml
components:
    elbEngine:
        type: elb-engine
    elbListener:
        type: elb-listener
name: provider-huawei-elb
```

**第二步**：在 `definition/versions.yaml` 中声明组件类型的版本和镜像：

```yaml
componentTypes:
    elb-engine:
        versions:
            - default: true
              image: example/elb-engine:1.0.0
              version: 1.0.0
    elb-listener:
        versions:
            - default: true
              image: example/elb-listener:1.0.0
              version: 1.0.0
```

**第三步**：在 `definition/components/types.go` 中定义组件的自定义配置结构体（CustomSpec）：

```go
// +k8s:openapi-gen=true
package components

// ElbEngineCustomSpec 定义 elb-engine 组件的自定义配置。
type ElbEngineCustomSpec struct {
    VpcID                string   `json:"vpcId"`
    VipSubnetCidrID      string   `json:"vipSubnetCidrId"`
    AvailabilityZoneList []string `json:"availabilityZoneList"`
}

// ElbListenerCustomSpec 定义 elb-listener 组件的自定义配置。
type ElbListenerCustomSpec struct {
    Protocol    string `json:"protocol"`
    Port        int32  `json:"port"`
    BackendPort int32  `json:"backendPort"`
}
```

> **设计决策**：本 Provider 使用 2 个组件（elbEngine + elbListener），将公网 IP / 带宽配置合并到拓扑级别（public-elb topology config），不再单独设置 elb-ip 组件。

### 4.2 定义拓扑（Topologies）

拓扑文件 `topology.yaml` 是**纯配置文件**（不是 Kubernetes CR），由 `provider-sdk generate` 读取并生成 OpenAPI schema。

在 `definition/topologies/public-elb/topology.yaml` 中定义公网 ELB 拓扑：

```yaml
config:
  configSchema: PublicElbTopologyConfig   # 引用 types.go 中的 Go 类型
  components:
    elbEngine:
      defaults:
        replicas: 1
    elbListener:
      optional: true

ui:
  sections:
    basicInfo:
      label: Basic Information
      components:
        version:
          uiType: select
          path: spec.components.elbEngine.version
          fieldParams:
            label: ELB Version
    network:
      label: Network
      components:
        vpcId:
          path: spec.components.elbEngine.customSpec.vpcId
          uiType: text
          fieldParams:
            label: VPC ID
            required: true
        # ... 其他字段
  sectionsOrder:
    - basicInfo
    - network
```

同时在 `definition/topologies/public-elb/types.go` 中定义拓扑配置类型：

```go
// +k8s:openapi-gen=true
package publicelb

type PublicElbTopologyConfig struct {
    BandwidthSize       int32  `json:"bandwidthSize,omitempty"`
    BandwidthChargeMode string `json:"bandwidthChargeMode,omitempty"`
    PublicIPNetworkType string `json:"publicIpNetworkType,omitempty"`
}
```

> **关键点**：
> - `topology.yaml` 不是 Kubernetes CR，没有 `apiVersion`/`kind` 字段
> - `config.configSchema` 引用 `types.go` 中的 Go 类型名，`provider-sdk generate` 会将其解析为 OpenAPI schema
> - `ui.sections` 定义前端表单，`path` 字段引用 Instance CR spec 中的字段路径
> - 拓扑目录名（如 `public-elb`）含连字符，但 Go 包名不含连字符（如 `publicelb`）

### 4.3 配置版本

见 4.1 第二步的 `versions.yaml` 示例。使用 `componentTypes` 作为顶层 key（不是 `components`），每个类型下有 `versions` 列表，其中恰好一个标记 `default: true`。

### 4.4 编写 UI Schema

UI Schema 直接在 `topology.yaml` 的 `ui` 部分定义（不是单独的 CR 字段）。支持的 `uiType` 包括：`text`、`number`、`select`、`group` 等。

```yaml
ui:
  sections:
    listener:
      label: Listener
      components:
        protocol:
          path: spec.components.elbListener.customSpec.protocol
          uiType: select
          fieldParams:
            label: Protocol
        port:
          path: spec.components.elbListener.customSpec.port
          uiType: number
          fieldParams:
            label: Port
          validation:
            min: 1
            max: 65535
  sectionsOrder:
    - basicInfo
    - network
    - listener
```

> **path 路径格式**：
> - 组件配置：`spec.components.<componentName>.customSpec.<field>`
> - 拓扑配置：`spec.topology.config.<field>`
> - 组件版本：`spec.components.<componentName>.version`


## 五、实现核心业务逻辑

### 5.1 实现 Provider 接口

Provider 接口定义在 `github.com/openeverest/openeverest/v2/provider-runtime/controller` 包中。四个方法的签名如下：

```go
// 实际的接口签名（不是 request/response 模式）
type ProviderInterface interface {
    Validate(c *Context) error
    Sync(c *Context) error
    Status(c *Context) (Status, error)
    Cleanup(c *Context) error
}
```

> **与早期文档的关键差异**：
> - 方法参数是 `*controller.Context`（不是 `context.Context` + request/response 结构体）
> - `Validate` 返回 `error`（不是 `*ValidateResponse`）
> - `Sync` 返回 `error`（不是 `*SyncResponse`）
> - `Status` 返回 `(controller.Status, error)`（不是 `*StatusResponse`）
> - `Cleanup` 返回 `error`（不是 `*CleanupResponse`）

在 `internal/provider/provider.go` 中实现：

```go
package provider

import (
    "k8s.io/apimachinery/pkg/runtime"
    "sigs.k8s.io/controller-runtime/pkg/log"

    "github.com/openeverest/openeverest/v2/provider-runtime/controller"
    "github.com/openeverest/provider-huawei-elb/internal/common"
    "github.com/openeverest/provider-huawei-elb/internal/huaweicloud"
)

var _ controller.ProviderInterface = (*Provider)(nil)

type Provider struct {
    controller.BaseProvider  // 必须嵌入 BaseProvider
}

func New() *Provider {
    return &Provider{
        BaseProvider: controller.BaseProvider{
            ProviderName: common.ProviderName,
            SchemeFuncs:  []func(*runtime.Scheme) error{},
            WatchConfigs: []controller.WatchConfig{},
        },
    }
}

func (p *Provider) Validate(c *controller.Context) error {
    cfg, err := ResolveConfig(c)
    if err != nil {
        return err
    }
    if cfg.VpcID == "" {
        return fmt.Errorf("elbEngine.customSpec.vpcId is required")
    }
    // ... 其他验证
    return nil
}

func (p *Provider) Sync(c *controller.Context) error {
    cfg, err := ResolveConfig(c)
    if err != nil { return err }

    creds, err := huaweicloud.LoadCredentials()
    if err != nil { return err }

    client, err := huaweicloud.NewELBClient(creds)
    if err != nil { return err }

    // 1. 检查 K8s Service 是否已有 ELB ID 注解
    // 2. 若无，按名称查找已有 ELB
    // 3. 若仍无，调用华为云 API 创建 ELB
    // 4. 创建/更新 K8s LoadBalancer Service（绑定 elb.id 注解）
    return nil
}

func (p *Provider) Status(c *controller.Context) (controller.Status, error) {
    // 查询 ELB 状态，返回 controller.Status
    // 就绪时使用 controller.ReadyWithConnectionDetails()
    // 创建中使用 controller.Provisioning("message")
    // 失败使用 controller.Failed("message")
}

func (p *Provider) Cleanup(c *controller.Context) error {
    // 1. 从 Service 注解获取 ELB ID
    // 2. 调用华为云 API 删除 ELB
    // 3. 删除 K8s Service
}
```

### 5.2 集成华为云 ELB SDK

> **关键修正**：ELB SDK 的包名是 `v3`（不是 `elb`），导入时必须使用别名。

```go
import (
    "github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
    elb "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3"
    elbregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/elb/v3/region"
)

func NewELBClient(creds *Credentials) (*elb.ElbClient, error) {
    auth := basic.NewCredentialsBuilder().
        WithAk(creds.AK).
        WithSk(creds.SK).
        WithProjectId(creds.ProjectID).
        Build()

    // region 必须通过 ELB region 包解析，不能直接传字符串
    reg := elbregion.ValueOf(creds.Region)
    if reg == nil {
        return nil, fmt.Errorf("unknown region: %s", creds.Region)
    }

    // SafeBuild() 返回 (*HcHttpClient, error)
    // Build() 返回 *HcHttpClient（不返回 error）
    hcClient, err := elb.ElbClientBuilder().
        WithCredential(auth).
        WithRegion(reg).
        SafeBuild()
    if err != nil {
        return nil, err
    }

    return elb.NewElbClient(hcClient), nil
}
```

> **与早期文档的差异**：
> - 包名是 `v3` 不是 `elb`，需要导入别名 `elb "...services/elb/v3"`
> - 客户端构造链：`ElbClientBuilder()`（不是 `NewClientBuilder()`）
> - `WithRegion()` 接收 `*region.Region`（不是字符串），需通过 `elbregion.ValueOf()` 解析
> - `Build()` / `SafeBuild()` 返回 `*HcHttpClient`，需要再用 `elb.NewElbClient(hcClient)` 包装

**凭证管理**：AK/SK/Region/ProjectID 通过环境变量注入（`HUAWEI_CLOUD_AK`、`HUAWEI_CLOUD_SK`、`HUAWEI_CLOUD_REGION`、`HUAWEI_CLOUD_PROJECT_ID`），在 Helm `values.yaml` 的 `extraEnv` 中配置。

### 5.3 管理 Kubernetes Service

通过 `controller.Context` 的 `Apply()` 方法管理 K8s 资源（自动设置 owner reference + create-or-update）：

```go
func EnsureService(c *controller.Context, cfg *ELBConfig, elbID string) error {
    svc := &corev1.Service{
        ObjectMeta: metav1.ObjectMeta{
            Name:      cfg.ELBName,           // 如 "elb-<instance-name>"
            Namespace: cfg.Namespace,         // Instance 所在命名空间
            Annotations: map[string]string{
                "kubernetes.io/elb.id": elbID, // 绑定预创建的 ELB
            },
        },
        Spec: corev1.ServiceSpec{
            Type: corev1.ServiceTypeLoadBalancer,
            Ports: []corev1.ServicePort{
                {
                    Protocol:   corev1.ProtocolTCP,
                    Port:       cfg.Port,
                    TargetPort: intstr.FromInt(int(cfg.BackendPort)),
                },
            },
            Selector: map[string]string{
                "openeverest.io/instance": cfg.InstanceName,
            },
        },
    }
    return c.Apply(svc)  // 自动 create-or-update + owner reference
}
```

> **与早期文档的差异**：
> - 使用 `c.Apply(svc)` 而非 `k8sClient.Create(ctx, svc)`——`Apply` 自动处理 create-or-update 和 owner reference
> - Namespace 使用 `c.Namespace()`（Instance 所在命名空间），不是硬编码 `"everest-system"`
> - CCE 通过 `kubernetes.io/elb.id` 注解自动管理 listener/pool，Provider 无需手动创建 listener

### 5.4 声明 RBAC 权限

在 `internal/provider/rbac.go` 中使用 Kubebuilder 标记声明所需权限：

```go
package provider

// Base RBAC（所有 Provider 必需）：
// +kubebuilder:rbac:groups=core.openeverest.io,resources=instances,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=core.openeverest.io,resources=instances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.openeverest.io,resources=instances/finalizers,verbs=update
// +kubebuilder:rbac:groups=core.openeverest.io,resources=providers,verbs=get;list;watch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Provider-specific RBAC：
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=endpoints,verbs=get;list;watch
```

运行 `make manifests` 后，`controller-gen` 会将这些标记生成为 `config/rbac/role.yaml`。


## 六、生成与部署

### 6.1 生成部署清单

```bash
make generate
```

此命令依次执行：
1. `make manifests` — 从 RBAC 标记生成 `config/rbac/role.yaml`
2. `make helm-sync-rbac` — 将 RBAC 规则同步到 `charts/provider-huawei-elb/generated/rbac-rules.yaml`
3. `go generate` — 运行 `provider-sdk generate`，从 `definition/` 文件生成 `charts/provider-huawei-elb/generated/provider-spec.yaml`

> **生成文件位置**：`charts/provider-huawei-elb/generated/`（不在项目根目录的 `generated/`）

### 6.2 本地运行测试

```bash
make run
```

在本地运行 Provider 并连接到测试 Kubernetes 集群。

### 6.3 构建镜像

```bash
docker build -t your-registry/provider-huawei-elb:latest .
docker push your-registry/provider-huawei-elb:latest
```

### 6.4 通过 Helm 部署

```bash
make helm-install
```

或手动安装：

```bash
helm install provider-huawei-elb ./charts/provider-huawei-elb \
  --namespace everest-system \
  --set image.repository=your-registry/provider-huawei-elb \
  --set image.tag=latest
```

**配置华为云凭证**：在 `values.yaml` 中通过 `extraEnv` 注入：

```yaml
extraEnv:
  - name: HUAWEI_CLOUD_AK
    valueFrom:
      secretKeyRef:
        name: huawei-cloud-credentials
        key: ak
  - name: HUAWEI_CLOUD_SK
    valueFrom:
      secretKeyRef:
        name: huawei-cloud-credentials
        key: sk
  - name: HUAWEI_CLOUD_REGION
    value: cn-north-4
  - name: HUAWEI_CLOUD_PROJECT_ID
    value: "your-project-id"
```

Providers 通过标准的 Helm 工作流进行安装和管理。


## 七、Provider 注册机制

### 7.1 注册原理

Provider 通过 Kubernetes 的 **Custom Resource Definition (CRD)** 机制向 OpenEverest 注册：

1. OpenEverest 在安装时已注册了 `Provider` 类型的 CRD（`apiVersion: core.openeverest.io/v1alpha1`）
2. `provider-sdk generate` 从 `definition/` 文件生成 Provider CR 的 spec（`provider-spec.yaml`）
3. Helm chart 的 `templates/provider.yaml` 将此 spec 应用为 Provider CR
4. OpenEverest 核心的控制器检测到新的 Provider CR 后自动完成发现和注册

> **修正**：Provider CR 的 `apiVersion` 是 `core.openeverest.io/v1alpha1`，不是 `plugin.openeverest.io/v1alpha1`。

### 7.2 Provider CR 结构

Provider CR 由 `make generate` 自动生成，**不需要手动编写**。其 spec 结构如下：

```yaml
apiVersion: core.openeverest.io/v1alpha1
kind: Provider
metadata:
  name: provider-huawei-elb
spec:
  componentTypes:
    elb-engine:
      versions:
        - default: true
          image: example/elb-engine:1.0.0
          version: 1.0.0
    elb-listener:
      versions:
        - default: true
          image: example/elb-listener:1.0.0
          version: 1.0.0
  components:
    elbEngine:
      type: elb-engine
    elbListener:
      type: elb-listener
  topologies:
    internal-elb:
      components:
        elbEngine: {}
        elbListener:
          optional: true
      configSchema:
        type: object
    public-elb:
      components:
        elbEngine: {}
        elbListener:
          optional: true
      configSchema:
        properties:
          bandwidthChargeMode:
            type: string
          bandwidthSize:
            format: int32
            type: integer
          publicIpNetworkType:
            type: string
        type: object
  uiSchema:
    # ... 各拓扑的 UI 表单定义
```

> **与早期文档的差异**：
> - `apiVersion` 是 `core.openeverest.io/v1alpha1`（不是 `plugin.openeverest.io/v1alpha1`）
> - spec 中没有 `displayName`、`description`、`type`、`providerImage` 等字段——这些是早期文档虚构的
> - spec 包含 `componentTypes`、`components`、`topologies`、`uiSchema` 字段，全部由 `provider-sdk generate` 从 `definition/` 文件自动生成


## 八、使用流程

### 8.1 用户视角

1. **选择 Provider**：用户在创建数据库时，在网络配置中选择"华为云 ELB"
2. **选择拓扑**：选择"公网负载均衡"或"私网负载均衡"
3. **填写配置**：根据 UI Schema 生成的表单填写带宽、VPC、子网等参数
4. **提交创建**：OpenEverest 核心将请求转发给 ELB Provider

### 8.2 平台调度流程

```
用户提交 Instance CR → OpenEverest 核心调和 → 调用 Provider.Sync(c) →
  ├─ 华为云 ELB API: 创建 ELB 实例
  └─ Kubernetes API: 创建 LoadBalancer Service（elb.id 注解绑定 ELB）
       ↓
Provider.Status(c) 返回 ConnectionDetails → 用户获得 ELB VIP:端口
```

### 8.3 Instance CR 示例

```yaml
apiVersion: core.openeverest.io/v1alpha1
kind: Instance
metadata:
  name: example-elb
spec:
  provider: provider-huawei-elb
  topology:
    type: public-elb
    config:
      bandwidthSize: 20
      bandwidthChargeMode: traffic
      publicIpNetworkType: 5_bgp
  components:
    elbEngine:
      type: elb-engine
      customSpec:
        vpcId: vpc-xxxxxxxx
        vipSubnetCidrId: subnet-xxxxxxxx
        availabilityZoneList:
          - cn-north-4a
          - cn-north-4b
    elbListener:
      type: elb-listener
      customSpec:
        protocol: TCP
        port: 3306
        backendPort: 3306
```


## 九、开发检查清单

| 阶段 | 任务 | 状态 |
|------|------|------|
| 环境 | 安装 Go 1.26+、Kubernetes 集群、OpenEverest | ☐ |
| 初始化 | `provider-sdk init` 创建项目 | ☐ |
| 定义 | `definition/provider.yaml` 声明组件 | ☐ |
| 定义 | `definition/versions.yaml` 配置版本/镜像 | ☐ |
| 定义 | `definition/components/types.go` 定义 CustomSpec | ☐ |
| 定义 | `definition/topologies/*/topology.yaml` + `types.go` | ☐ |
| 实现 | `internal/huaweicloud/client.go` ELB 客户端 | ☐ |
| 实现 | `internal/huaweicloud/elb.go` ELB CRUD | ☐ |
| 实现 | `internal/provider/config.go` 配置解析 | ☐ |
| 实现 | `internal/provider/service.go` K8s Service 管理 | ☐ |
| 实现 | `internal/provider/provider.go` Validate/Sync/Status/Cleanup | ☐ |
| 实现 | `internal/provider/rbac.go` RBAC 标记 | ☐ |
| 生成 | `make generate` 生成清单 | ☐ |
| 验证 | `go build ./...` + `go vet ./...` | ☐ |
| 测试 | `make run` 本地测试 | ☐ |
| 部署 | 构建镜像并 Helm 安装 | ☐ |


## 十、参考资源

- **Provider SDK**：https://github.com/openeverest/provider-sdk
- **官方 Provider 开发文档**：https://github.com/openeverest/provider-sdk/blob/main/PROVIDER_DEVELOPMENT.md
- **MongoDB Provider 参考实现**：https://github.com/openeverest/plugin-mongodb-explorer
- **ClickHouse Provider 社区实现**：https://github.com/scaledb-io/provider-altinity-clickhouse
- **OpenEverest 主仓库**：https://github.com/openeverest/openeverest
- **社区交流**：Slack 或 GitHub Discussions
- **OpenEverest 公开路线图**：https://github.com/openeverest/roadmap


## 十一、注意事项

1. **OpenEverest v2 目前是 Developer Preview**：`v2.0.0-dev.1` 是早期预览版，不建议用于生产环境
2. **API 可能会有变化**：在正式发布（GA）之前可能会有 breaking changes
3. **v1 和 v2 不兼容**：两个版本架构和数据模型完全不同，不支持在同一集群中同时运行
4. **提交代码需签署 DCO**：每个 commit 必须包含 `Signed-off-by` 签名
5. **华为云 ELB API 版本**：使用 ELB v3 API（`services/elb/v3`），不使用 v2
6. **信任上游源码**：当本指南与上游仓库行为不一致时，以上游仓库源码为准
7. **provider-sdk 生成的 Go 标识符可能有 bug**：`provider-sdk add component` 会将含连字符的类型名（如 `elb-engine`）直接用作 Go 标识符（如 `Elb-engineCustomSpec`），这是非法的 Go 标识符。需手动修复为驼峰命名（如 `ElbEngineCustomSpec`）。
8. **provider-sdk add topology 在非 TTY 环境失败**：`provider-sdk add topology` 使用交互式选择器，在非 TTY 环境（如 CI、Agent）中会报 `could not open a new TTY` 错误。需手动创建拓扑目录和文件。
