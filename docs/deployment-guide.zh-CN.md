# OpenEverest + provider-huawei-elb 安装部署指南

[English](deployment-guide.md) | 中文

本文档面向初学者，从零开始在 Linux 环境部署 OpenEverest 平台并集成华为云 ELB Provider 插件，实现数据库实例自动绑定华为云 ELB。

> **注意**：OpenEverest v2 目前处于 Developer Preview 阶段（`v2.0.0-dev.1`），API 可能在 GA 前变更。v1 与 v2 不兼容，请勿在同一集群中混用。

---

## 一、前提条件

### 1.1 部署位置概览

整个部署涉及三个位置，明确你“在哪里操作什么”：

```
┌─────────────────────────────────────────────────────────────┐
│  你的 Mac（本地机器）                                         │
│  ────────────────────                                        │
│  • 执行所有命令的地方（kubectl / helm / docker / git）       │
│  • 不运行任何服务，只是“指挥台”                               │
│  • 需要能上网（访问公网）和访问 CCE API Server                │
└──────────┬──────────────────────────┬──────────────────────┘
           │                          │
           │ 1. docker push           │ 2. helm install / kubectl apply
           │   推送镜像               │   部署 + 创建资源
           ▼                          ▼
┌──────────────────────┐   ┌──────────────────────────────────┐
│  华为云 SWR           │   │  华为云 CCE 集群                  │
│  ────────────         │   │  ──────────────                   │
│  • 存放容器镜像        │   │  • 运行 OpenEverest 平台          │
│  • 被 CCE 节点拉取     │   │  • 运行 Provider 容器（7×24）     │
│                       │   │  • Provider 调华为云 ELB API      │
│                       │   │    创建/删除 ELB 实例              │
└──────────────────────┘   └──────────────────────────────────┘
```

**关键**：你的 Mac 只负责“下达命令”，真正运行服务的是 CCE 集群。Mac 关机后 Provider 依然在 CCE 里运行。

### 1.2 工具与文件下载

需要在 Mac 上安装以下工具，并从 CCE 控制台下载一个集群凭证文件：

| 工具/文件 | 用途 | 下载方式 |
|---|---|---|
| **kubectl** | 操作 K8s 集群的命令行工具 | `brew install kubectl` 或 [官方指南](https://kubernetes.io/zh-cn/docs/tasks/tools/) |
| **helm** | 部署 Helm Chart（OpenEverest + Provider） | `brew install helm` 或 [官方指南](https://helm.sh/zh/docs/intro/install/) |
| **docker** | 构建容器镜像 | 安装 [Docker Desktop](https://www.docker.com/products/docker-desktop/) |
| **git** | 克隆插件仓库 | 系统自带或 `brew install git` |
| **kubeconfig** | CCE 集群连接凭证（含 API Server 地址 + 客户端证书/私钥） | 华为云 CCE 控制台下载，见 §1.3 |

> Mac 用户推荐先装 [Homebrew](https://brew.sh/)，再用 `brew install kubectl helm git` 一次性装好前三个。

### 1.3 下载文件存放位置

| 文件 | 存放位置 | 说明 |
|---|---|---|
| kubectl / helm / docker / git | 系统 PATH（`/opt/homebrew/bin/` 或 `/usr/local/bin/`） | brew 安装后自动放好，终端可直接调用 |
| **kubeconfig** | `~/.kube/config` | kubectl 默认读取此路径，放别处需设 `KUBECONFIG` 环境变量 |
| 插件仓库代码 | 任意目录，如 `~/projects/provider-huawei-elb` | `git clone` 后即可（§二） |
| `provider-values.yaml` | 仓库根目录（与 Makefile 同级） | §4.4 会创建，不用提前准备 |

**下载 kubeconfig 的具体步骤**：

1. 登录 [华为云控制台](https://console.huaweicloud.com/) → **云容器引擎 CCE** → 你的集群
2. 点击集群名称进入详情页
3. 右上角 **连接信息** → **kubectl** 页签
4. 点击 **下载** 获取 kubeconfig 文件（通常下载到 `~/Downloads/`）

**放置 kubeconfig 到 kubectl 默认读取位置**：

```bash
# 1. 创建 .kube 目录（如果没有）
mkdir -p ~/.kube

# 2. 把下载的 kubeconfig 文件移动过去并重命名为 config
mv ~/Downloads/kubeconfig.json ~/.kube/config

# 3. 设置权限（保护里面的私钥，不设会被 kubectl 拒绝读取）
chmod 600 ~/.kube/config

# 4. 验证连通性 —— 能返回节点列表说明凭证生效
kubectl get nodes
```

> kubeconfig 里包含 CCE API Server 的公网地址和你的客户端证书/私钥，相当于“集群钥匙”。务必保管好，**不要提交到 Git 或分享给他人**。

### 1.4 软件与账号要求

| 项目 | 要求 |
|---|---|
| Kubernetes 集群 | 1.24+（华为云 CCE 或其他标准 K8s） |
| kubectl | 已安装并配置集群访问权限 |
| Helm | v3.x（[安装指南](https://helm.sh/docs/intro/install/)） |
| Git | 已安装（克隆插件仓库用） |
| 华为云账号 | 具备 ELB 创建权限，获取 AK / SK / Region / Project ID |
| 容器镜像仓库 | 已推送 `provider-huawei-elb` 镜像（本文使用华为云 SWR） |
| 网络连通 | 执行机器可访问公网（拉取 Chart / 镜像）和 K8s API Server |

验证集群连通性：

```bash
kubectl get nodes
```

---

## 二、克隆插件仓库与目录结构

### 2.1 克隆仓库

```bash
git clone https://github.com/weimantian/provider-huawei-elb.git
cd provider-huawei-elb
```

> 仓库为私有，需先配置 GitHub 访问权限（SSH key 或 Personal Access Token）。

### 2.2 目录结构速览

以下是仓库中与部署相关的关键目录和文件，**后续步骤会频繁引用这些路径**：

```
provider-huawei-elb/                        ← 仓库根目录（你当前所在位置）
│
├── charts/
│   └── provider-huawei-elb/                ← Helm Chart 目录（部署核心）
│       ├── values.yaml                     ← 默认配置（❌ 不要直接改，用 --values 覆盖）
│       ├── Chart.yaml                      ← Chart 元数据
│       ├── templates/                      ← K8s 资源模板（Helm 渲染用，无需手动改）
│       │   ├── deployment.yaml             ← Provider Pod 模板
│       │   ├── service.yaml                ← Service 模板
│       │   ├── provider.yaml               ← Provider CR 模板
│       │   ├── clusterrole.yaml            ← RBAC 模板
│       │   ├── clusterrolebinding.yaml     ← RBAC 模板
│       │   └── serviceaccount.yaml         ← SA 模板
│       └── generated/                      ← 自动生成的文件（✋ 切勿手动编辑）
│           ├── provider-spec.yaml          ← Provider CR 的 spec（make generate 生成）
│           └── rbac-rules.yaml             ← RBAC 规则（make generate 生成）
│
├── examples/                               ← 示例 Instance CR（✅ 可直接用或修改后用）
│   ├── instance-simple.yaml                ← 最简公网 ELB（默认 TCP:3306）
│   ├── instance-example.yaml               ← 完整公网 ELB（含自定义监听器）
│   └── instance-internal-elb.yaml          ← 内网 ELB
│
├── config/
│   └── rbac/
│       └── role.yaml                       ← ClusterRole（make generate 生成）
│
├── Dockerfile                              ← 标准构建（需网络拉 golang:1.26）
├── Dockerfile.local                        ← 本地构建（无需网络，交叉编译）
├── Makefile                                ← 构建命令集合
│
└── provider-values.yaml                    ← ⚠️ 你需要自己创建的文件（本指南 §四.4）
```

**关键区分**：
- ✅ `examples/` 下的文件 → 仓库自带，修改后直接 `kubectl apply`
- ⚠️ `provider-values.yaml` → **你需要在仓库根目录手动创建**（本指南会提供完整内容）
- ✋ `charts/.../generated/` 和 `config/rbac/` → 自动生成，切勿手动编辑

---

## 三、部署 OpenEverest 平台

### 3.1 添加 Helm 仓库

```bash
helm repo add openeverest https://openeverest.github.io/helm-charts/
helm repo update
```

> 如果上述仓库地址不可用，请参考 [OpenEverest 官方仓库](https://github.com/openeverest/openeverest) 获取最新 Chart 地址。

### 3.2 安装 OpenEverest 核心

```bash
helm install everest-core openeverest/openeverest \
  --namespace everest-system \
  --create-namespace
```

该命令在 `everest-system` 命名空间部署 OpenEverest Server、Operator 及依赖组件。

### 3.3 验证部署状态

```bash
kubectl get pods -n everest-system
```

所有 Pod 应为 `Running` 状态。若有 Pod 未就绪，等待 1-2 分钟后重试。

### 3.4 获取管理员密码（可选）

```bash
kubectl get secret everest-accounts -n everest-system \
  -o jsonpath='{.data.users\.yaml}' | base64 --decode
```

> Secret 名称和字段路径可能因 OpenEverest 版本而异。若上述命令失败，请查阅 [OpenEverest 官方文档](https://github.com/openeverest/openeverest)。

### 3.5 访问 UI（可选）

```bash
kubectl port-forward svc/everest 8080:8080 -n everest-system
```

浏览器访问 `http://127.0.0.1:8080`，使用 `admin` 和上一步获取的密码登录。

> 生产环境建议通过 Ingress 或 LoadBalancer 暴露 UI。

---

## 四、部署 provider-huawei-elb 插件

> **说明**：本章节所有操作都在你的 **Linux 本地机器**上执行（安装了 `kubectl` 和 `helm` 的机器），**不需要进入容器**。`provider-values.yaml` 是本地文件，Helm 读取它后部署到集群。后续修改配置也是改本地文件后 `helm upgrade`，无需 `docker exec` 或 `kubectl exec` 进入容器。

### 4.1 镜像准备

部署 Provider 需要一个容器镜像。有两种方式获取，**任选其一**即可。

#### 方式一：使用已构建好的多架构镜像（推荐，直接拉取）

镜像已构建并推送到华为云 SWR，**同时支持 amd64（x86）和 arm64（鲲鹏）架构**，直接拉取即可：

```bash
docker pull swr.cn-north-4.myhuaweicloud.com/weimantian/provider-huawei-elb:latest
```

拉取成功后跳到 [§4.2](#42-创建华为云凭证-secret) 继续部署。

#### 方式二：本地构建镜像

如果需要修改代码后重新构建，或无法访问 SWR，可以在**仓库根目录**自行构建。

##### 方式 A：标准构建（需要网络，Docker 自动拉取 golang 基础镜像）

```bash
# 在仓库根目录执行，使用标准 Dockerfile
docker build -t provider-huawei-elb:latest .
```

> 此方式使用 `Dockerfile`，Docker 会自动拉取 `golang:1.26` 编译 Go 源码，再用 `distroless` 打包运行时镜像。需要能访问 Docker Hub。
>
> **注意**：此方式只构建当前机器架构的单架构镜像。

##### 方式 B：本地交叉编译构建（无需网络，使用本地基础镜像）

如果机器无法访问 Docker Hub（拉不到 `golang:1.26`），先交叉编译二进制，再用本地已有的基础镜像打包：

```bash
# 1. 确认集群架构（arm64 或 amd64）
kubectl get nodes -o jsonpath='{.items[0].status.nodeInfo.architecture}'

# 2. 交叉编译 Go 二进制（根据集群架构选择 GOARCH）
#    arm64 集群（如华为云鲲鹏 CCE）：
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -o bin/provider cmd/provider/main.go

#    amd64 集群（如华为云 x86 CCE）：
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o bin/provider cmd/provider/main.go

# 3. 用 Dockerfile.local 打包（禁用 BuildKit 避免 SWR 不兼容的 manifest list）
DOCKER_BUILDKIT=0 docker build -f Dockerfile.local -t provider-huawei-elb:latest .
```

> 此方式使用 `Dockerfile.local`，以本地 `redis:7-alpine` 镜像为基础（含 CA 证书，支持 HTTPS 调华为云 API）。二进制是静态编译的，无 CGO 依赖。
>
> **注意**：此方式只构建指定架构的单架构镜像。

##### 方式 C：多架构构建（同时支持 amd64 + arm64）

如果需要同时支持 x86 和鲲鹏集群，构建一个多架构镜像（manifest list）：

```bash
# 1. 交叉编译两种架构的二进制
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -a -o bin/provider-arm64 cmd/provider/main.go
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o bin/provider-amd64 cmd/provider/main.go

# 2. 构建 arm64 镜像（--provenance=false 避免 SWR 不兼容 attestation）
docker buildx build --platform linux/arm64 --provenance=false --load \
  -t swr.cn-north-4.myhuaweicloud.com/weimantian/provider-huawei-elb:latest-arm64 \
  -f Dockerfile.multiarch .

# 3. 构建 amd64 镜像
docker buildx build --platform linux/amd64 --provenance=false --load \
  -t swr.cn-north-4.myhuaweicloud.com/weimantian/provider-huawei-elb:latest-amd64 \
  -f Dockerfile.multiarch .

# 4. 登录 SWR
docker login -u cn-north-4@<YOUR_ACCESS_KEY> -p <YOUR_LOGIN_TOKEN> swr.cn-north-4.myhuaweicloud.com

# 5. 推送两个单架构镜像
docker push swr.cn-north-4.myhuaweicloud.com/weimantian/provider-huawei-elb:latest-arm64
docker push swr.cn-north-4.myhuaweicloud.com/weimantian/provider-huawei-elb:latest-amd64

# 6. 合并为多架构 manifest list 并推送
docker manifest create \
  swr.cn-north-4.myhuaweicloud.com/weimantian/provider-huawei-elb:latest \
  swr.cn-north-4.myhuaweicloud.com/weimantian/provider-huawei-elb:latest-arm64 \
  swr.cn-north-4.myhuaweicloud.com/weimantian/provider-huawei-elb:latest-amd64
docker manifest push swr.cn-north-4.myhuaweicloud.com/weimantian/provider-huawei-elb:latest
```

> 此方式使用 `Dockerfile.multiarch`，通过 `TARGETARCH` 自动选择对应架构的二进制。
>
> **关键**：`--provenance=false` 是必须的，否则 BuildKit 生成的 attestation 元数据会被 SWR 拒绝（报错 `Invalid image, fail to parse 'manifest.json'`）。

##### 构建完成后：根据集群类型选择

**场景 A：部署到远程 CCE 集群**（CCE 节点无法访问你本地的 Docker 镜像，必须推送到 SWR）

如果使用方式 A/B（单架构），需手动推送到 SWR：

```bash
# 1. 登录华为云 SWR（在华为云控制台 → 容器镜像服务 SWR → 登录指令中获取）
docker login -u cn-north-4@<YOUR_ACCESS_KEY> -p <YOUR_LOGIN_TOKEN> swr.cn-north-4.myhuaweicloud.com

# 2. 打标签（替换 weimantian 为你的 SWR 组织/命名空间）
docker tag provider-huawei-elb:latest swr.cn-north-4.myhuaweicloud.com/weimantian/provider-huawei-elb:latest

# 3. 推送
docker push swr.cn-north-4.myhuaweicloud.com/weimantian/provider-huawei-elb:latest
```

如果使用方式 C（多架构），镜像已在构建过程中推送，跳过此步。

然后在 `provider-values.yaml` 中使用 SWR 镜像地址：
```yaml
image:
  repository: swr.cn-north-4.myhuaweicloud.com/weimantian/provider-huawei-elb
  tag: latest
  pullPolicy: IfNotPresent
```

**场景 B：本地 k3d/kind 开发集群**（K8s 节点就是本机，直接用本地镜像，无需推送）

```bash
# k3d：将本地镜像导入集群
k3d image import provider-huawei-elb:latest

# kind：将本地镜像导入集群
kind load docker-image provider-huawei-elb:latest
```

然后在 `provider-values.yaml` 中使用本地镜像名，并设为永不拉取：
```yaml
image:
  repository: provider-huawei-elb
  tag: latest
  pullPolicy: Never          # 本地开发用，不从仓库拉取
```

### 4.2 创建华为云凭证 Secret

**不要通过 `--set` 命令行传递 AK/SK**（会暴露在 shell 历史中）。使用 Kubernetes Secret：

```bash
kubectl create secret generic huawei-cloud-credentials \
  --from-literal=ak=<YOUR_AK> \
  --from-literal=sk=<YOUR_SK> \
  --from-literal=project-id=<YOUR_PROJECT_ID> \
  -n everest-system
```

### 4.3 创建 SWR 拉取凭证（私有仓库用）

如果 SWR 仓库是私有的，需创建拉取凭证：

```bash
kubectl create secret docker-registry swr-pull-secret \
  --docker-server=swr.cn-north-4.myhuaweicloud.com \
  --docker-username=cn-north-4@<YOUR_ACCESS_KEY> \
  --docker-password=<YOUR_LOGIN_TOKEN> \
  -n everest-system
```

> 如果仓库是公开的，跳过此步骤，并在下面的 `provider-values.yaml` 中删掉 `imagePullSecrets` 部分。

### 4.4 创建 provider-values.yaml

在**仓库根目录**（与 `Makefile` 同级）创建 `provider-values.yaml`：

```bash
cat > provider-values.yaml <<'EOF'
image:
  repository: swr.cn-north-4.myhuaweicloud.com/weimantian/provider-huawei-elb
  tag: latest
  pullPolicy: IfNotPresent

# SWR 私有仓库拉取凭证（仓库公开则删掉这两行）
imagePullSecrets:
  - name: swr-pull-secret

# 华为云凭证通过环境变量注入
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
    valueFrom:
      secretKeyRef:
        name: huawei-cloud-credentials
        key: project-id
EOF
```

> **这个文件是你唯一需要手动创建的文件**。它覆盖了 `charts/provider-huawei-elb/values.yaml` 中的默认值（镜像地址 + 凭证）。
>
> **关键**：Provider Helm Chart **不提供 `config.ak/sk` 等字段**。所有华为云凭证通过 `extraEnv` 环境变量注入，支持 `value`（明文）和 `valueFrom.secretKeyRef`（Secret 引用）两种方式。

### 4.5 安装 Provider

> **这一步会自动启动容器**：`helm install` 创建 Kubernetes Deployment，K8s 自动从 SWR 拉取镜像并在集群中启动 Provider 容器。你不需要手动 `docker run`。
在**仓库根目录**执行：

```bash
helm install provider-huawei-elb ./charts/provider-huawei-elb \
  --namespace everest-system \
  --values provider-values.yaml
```

Helm 会同时部署：
- **Deployment**（Provider Pod）
- **Provider CR**（从 `charts/provider-huawei-elb/generated/provider-spec.yaml` 读取 spec）
- **ClusterRole / ClusterRoleBinding / ServiceAccount**（RBAC 权限）
- **Service**（HTTP 端口 8082，用于 schema 查询和健康检查）

### 4.6 验证插件部署

```bash
# 1. Provider Pod 状态
kubectl get pods -n everest-system -l app.kubernetes.io/name=provider-huawei-elb

# 2. Provider CR 是否创建
kubectl get provider provider-huawei-elb -n everest-system

# 3. 查看 Provider 日志（确认凭证加载成功、无报错）
kubectl logs -n everest-system -l app.kubernetes.io/name=provider-huawei-elb --tail=30
```

Pod 状态为 `Running` 且日志无 `ERROR` 级别输出即为部署成功。

---

## 五、创建 ELB 实例

仓库 `examples/` 目录下有 3 个现成的示例文件，修改后即可使用。

### 5.1 公网 ELB（带 EIP，外网可访问）

使用 `examples/instance-simple.yaml`：

```bash
# 1. 编辑文件，替换 VPC / 子网 / 可用区为你的实际华为云资源 ID
vi examples/instance-simple.yaml
```

需要修改的字段（`examples/instance-simple.yaml`）：

```yaml
apiVersion: core.openeverest.io/v1alpha1
kind: Instance
metadata:
  name: my-elb-simple
  namespace: everest-system          # ← 添加命名空间
spec:
  provider: provider-huawei-elb
  topology:
    type: public-elb
    config:
      bandwidthSize: 10              # 带宽 Mbit/s
      bandwidthChargeMode: traffic
  components:
    elbEngine:
      type: elb-engine
      customSpec:
        vpcId: vpc-xxxxxxxx          # ← 替换为你的 VPC ID
        vipSubnetCidrId: subnet-xxxxxxxx  # ← 替换为你的子网 ID
        availabilityZoneList:
          - cn-north-4a              # ← 替换为你的可用区
```

```bash
# 2. 应用
kubectl apply -f examples/instance-simple.yaml
```

### 5.2 内网 ELB（仅 VPC 内可访问）

使用 `examples/instance-internal-elb.yaml`：

```bash
# 1. 编辑文件，替换 VPC / 子网 / 可用区
vi examples/instance-internal-elb.yaml
```

该文件内容（`examples/instance-internal-elb.yaml`）：

```yaml
apiVersion: core.openeverest.io/v1alpha1
kind: Instance
metadata:
  name: my-internal-elb
  namespace: everest-system          # ← 添加命名空间
spec:
  provider: provider-huawei-elb
  topology:
    type: internal-elb               # 内网拓扑，无需 bandwidth 配置
  components:
    elbEngine:
      type: elb-engine
      customSpec:
        vpcId: vpc-xxxxxxxx          # ← 替换
        vipSubnetCidrId: subnet-xxxxxxxx  # ← 替换
        availabilityZoneList:
          - cn-north-4a
    elbListener:
      type: elb-listener
      customSpec:
        protocol: TCP
        port: 3306
        backendPort: 3306
```

```bash
# 2. 应用
kubectl apply -f examples/instance-internal-elb.yaml
```

### 5.3 完整配置公网 ELB（自定义监听器）

使用 `examples/instance-example.yaml`，支持双可用区 + 自定义监听器端口：

```bash
vi examples/instance-example.yaml    # 替换 VPC / 子网 / 可用区
kubectl apply -f examples/instance-example.yaml
```

> **省略 `elbListener` 时**使用默认值：协议 TCP、端口 3306、后端端口 3306。
>
> **协议说明**：TCP/UDP 直接透传；HTTP/HTTPS 在 ELB 层做 L7，K8s Service 协议回退为 TCP。

---

## 六、验证 ELB 与数据库访问

### 6.1 查看 Instance 状态

```bash
kubectl get instance -n everest-system
```

状态变化：`Provisioning` → `Ready`（或 `Failed`）。

```bash
# 查看详细状态，包含 ELB IP 和端口
kubectl get instance my-elb-simple -n everest-system -o yaml
```

关注 `status` 字段：

```yaml
status:
  provisioningStatus: Ready
  connectionDetails:
    host: <ELB_PUBLIC_IP>      # 公网 ELB 为 EIP；内网 ELB 为私网 IP
    port: "3306"
```

### 6.2 查看 K8s Service

Provider 创建的 Service 命名为 `elb-<instance-name>`：

```bash
kubectl get svc -n everest-system
```

```
NAME                    TYPE           CLUSTER-IP    EXTERNAL-IP      PORT(S)          AGE
elb-my-elb-simple       LoadBalancer   10.x.x.x      <ELB_PUBLIC_IP>  3306:3xxxx/TCP   2m
```

确认 `EXTERNAL-IP` 已分配（公网 ELB 显示 EIP，内网 ELB 显示私网 IP）。

查看 Service 注解（确认 ELB ID 绑定）：

```bash
kubectl get svc elb-my-elb-simple -n everest-system -o jsonpath='{.metadata.annotations}' | jq .
```

```json
{
  "kubernetes.io/elb.id": "<ELB_ID>"
}
```

### 6.3 验证 ELB 状态（华为云控制台）

1. 登录 [华为云控制台](https://console.huaweicloud.com/) → **弹性负载均衡 ELB**
2. 找到名称包含 `elb-my-elb-simple` 的实例
3. 确认状态为 **运行中**（ACTIVE）
4. 检查 **监听器** 页签：应有 TCP:3306 监听器
5. 检查 **后端服务器组**：后端成员健康状态应为 **正常**

### 6.4 测试网络连通性

**公网 ELB**（从外网测试）：

```bash
# 获取 ELB 公网 IP
ELB_IP=$(kubectl get instance my-elb-simple -n everest-system \
  -o jsonpath='{.status.connectionDetails.host}')
echo "ELB IP: $ELB_IP"

# TCP 端口连通性测试
nc -zv $ELB_IP 3306
```

**内网 ELB**（从 VPC 内的机器测试）：

```bash
nc -zv <ELB_PRIVATE_IP> 3306
```

预期输出 `Connection to <IP> 3306 port [tcp/mysql] succeeded!`。

### 6.5 测试数据库访问

假设后端数据库为 MySQL（端口 3306）：

```bash
# 公网 ELB
mysql -h $ELB_IP -P 3306 -u <DB_USER> -p<DB_PASSWORD> -e "SELECT 1"

# 内网 ELB（需在 VPC 内执行）
mysql -h <ELB_PRIVATE_IP> -P 3306 -u <DB_USER> -p<DB_PASSWORD> -e "SELECT 1"
```

PostgreSQL（端口 5432）：

```bash
psql -h <ELB_IP> -p 5432 -U <DB_USER> -d <DB_NAME> -c "SELECT 1"
```

预期输出 `1` 或类似结果即为数据库访问正常。

### 6.6 ELB 健康检查验证

ELB 会自动对后端数据库做健康检查。若后端不可达，ELB 会标记成员异常：

```bash
# 通过华为云 CLI 查看 ELB 后端健康状态
# 安装：https://support.huaweicloud.com/ptrc-hcli/hcli_01_01.html
hcloud ELB ShowMemberHealth --elb_id=<ELB_ID>
```

或在华为云控制台 → ELB 实例 → **后端服务器组** → 查看成员健康状态。

> **健康检查未就绪**：ELB 创建后需 10-30 秒完成健康检查。在此期间连接会超时，属正常现象。

---

## 七、故障排查

| 症状 | 排查方法 |
|---|---|
| Provider Pod `CrashLoopBackOff` | `kubectl logs -n everest-system <pod>` 查看日志。常见：凭证缺失 / Region 错误 / 镜像拉取失败 |
| Instance 卡在 `Provisioning` | `kubectl describe instance <name> -n everest-system` 查看事件。检查 VPC/子网 ID 是否正确 |
| Instance 状态为 `Failed` | Provider 日志会有详细错误。常见：ELB 配额已满 / 子网无可用 IP / 可用区不支持 |
| Service 无 `EXTERNAL-IP` | 公网 ELB：检查 EIP 配额。内网 ELB：检查子网 CIDR 是否冲突。CCE 控制器处理需 1-3 分钟 |
| ELB 后端成员异常 | 确认数据库 Pod 正在运行、监听端口与 `backendPort` 一致、安全组放行 ELB 到数据库的流量 |
| 数据库连接超时 | 确认 ELB 健康检查已通过（§6.6）、安全组规则放行客户端到 ELB 的入站流量 |
| `helm install` 报 `connection refused` | 检查 kubeconfig：`kubectl cluster-info` |
| `helm install` 找不到 chart | 确认在仓库根目录执行，路径 `./charts/provider-huawei-elb` 存在 |

---

## 八、清理资源

### 删除单个 ELB 实例

```bash
kubectl delete instance my-elb-simple -n everest-system
```

Provider 会自动删除对应的华为云 ELB 和 K8s Service。

### 卸载 Provider

```bash
helm uninstall provider-huawei-elb -n everest-system
```

### 卸载 OpenEverest

```bash
helm uninstall everest-core -n everest-system
kubectl delete namespace everest-system
```

> **注意**：删除 Instance CR 后，Provider 会先删除华为云 ELB 再删除 K8s Service。若 Provider 已卸载，需手动在华为云控制台删除残留的 ELB。

---

## 九、参考资源

- [OpenEverest 官方仓库](https://github.com/openeverest/openeverest)
- [Provider SDK 文档](https://github.com/openeverest/provider-sdk/blob/main/PROVIDER_DEVELOPMENT.md)
- [华为云 ELB 文档](https://support.huaweicloud.com/elb/)
- [华为云 ELB Go SDK](https://github.com/huaweicloud/huaweicloud-sdk-go-v3)
- [CCE ELB 注解说明](https://support.huaweicloud.com/usermanual-cce/cce_10_0385.html)
- 本项目 GitHub 仓库：https://github.com/weimantian/provider-huawei-elb
