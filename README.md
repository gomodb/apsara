# Apsara SDK — 阿里云专有云通用 Go SDK

[![Go Reference](https://pkg.go.dev/badge/github.com/gomodb/apsara.svg)](https://pkg.go.dev/github.com/gomodb/apsara)

通用、轻量的阿里云专有云（Apsara V3.18.6）Go SDK，**不定义任何 API 的业务 struct**，仅提供认证、请求构建和响应反序列化的通用基础设施。

- **通用**：一个 Client 实例可调用任意产品的任意 API
- **零耦合**：不生成任何 API struct，响应用 `map[string]any` 或自定义类型接收
- **函数选项模式**：`NewClient(opts...)`，扩展不破坏兼容性
- **凭证链**：自动从环境变量加载，也可手动指定
- **结构化错误**：`ApsaraError` 包含状态码、RequestId、业务错误码
- **指数退避重试**：可配置重试次数，内置 jitter
- **响应元数据**：通过 `WithMeta` 获取 RequestId、状态码、原始 Header
- **专有云适配**：内置 `x-acs-organizationid`、`x-acs-resourcegroupid` 等 Header
- **自签名证书**：支持 `WithInsecureSkipVerify`，适配内网环境
- **轻量依赖**：零外部依赖：仅使用 `Go` 标准库

---

## 安装

```bash
go get github.com/gomodb/apsara
```

## 快速开始

### 方式一：手动指定凭证

```go
package main

import (
    "context"
    "fmt"
    "os"

    "github.com/gomodb/apsara"
)

func main() {
    ctx := context.Background()

    client, err := apsara.NewClient(
        apsara.WithEndpoint("ecs.aliyuncs.com"),
        apsara.WithRegion("cn-hangzhou"),
        apsara.WithCredential(apsara.Credential{
            AccessKeyID:     os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID"),
            AccessKeySecret: os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET"),
        }),
    )
    if err != nil {
        panic(err)
    }

    var resp map[string]any
    err = client.Get(ctx, "DescribeInstances", "2014-05-26",
        map[string]string{"PageSize": "5"}, &resp)
    if err != nil {
        panic(err)
    }
    fmt.Printf("Total: %v\n", resp["TotalCount"])
}
```

### 方式二：从环境变量加载凭证

```bash
export APSARA_ACCESS_KEY_ID=your-access-key-id
export APSARA_ACCESS_KEY_SECRET=your-access-key-secret
export APSARA_SECURITY_TOKEN=your-sts-token   # STS 可选
export APSARA_ENDPOINT=ecs.aliyuncs.com
export APSARA_REGION_ID=cn-hangzhou
```

```go
client, err := apsara.NewClient(  // 从环境变量自动读取
        apsara.WithTimeout(30*time.Second),
    )
```

### 方式三：专有云完整配置

```go
client, err := apsara.NewClient(
    apsara.WithEndpoint("ecs.aliyuncs.com"),
    apsara.WithRegion("cn-hangzhou"),
    apsara.WithCredential(apsara.Credential{
        AccessKeyID:     "xxx",
        AccessKeySecret: "yyy",
    }),
    apsara.WithInsecureSkipVerify(true),   // 自签名证书
    apsara.WithOrganizationID("org-xxx"),
    apsara.WithResourceGroupID("rg-xxx"),
    apsara.WithCallerSource("my-app"),
    apsara.WithTimeout(30*time.Second),
    apsara.WithRetry(3),                   // 失败重试 3 次
)
```

## 调用 API

```go
// GET 请求
client.Get(ctx, "DescribeInstances", "2014-05-26",
    map[string]string{"PageSize": "10"}, &result)

// POST 请求
client.Post(ctx, "CreateInstance", "2014-05-26",
    map[string]string{"ImageId": "centos_7_9_x64", "InstanceType": "ecs.g6.large"},
    &result)

// 获取响应元数据
var meta apsara.ResponseMeta
client.Get(ctx, "DescribeInstances", "2014-05-26", nil, &result, apsara.WithMeta(&meta))
fmt.Println("RequestId:", meta.RequestID)
fmt.Println("StatusCode:", meta.StatusCode)
```

## 错误处理

```go
err := client.Get(ctx, "DescribeInstances", "2014-05-26", nil, &result)
if err != nil {
    var ae *apsara.ApsaraError
    if errors.As(err, &ae) {
        fmt.Printf("Status: %d\n", ae.StatusCode)
        fmt.Printf("RequestId: %s\n", ae.RequestID)
        fmt.Printf("ErrorCode: %s\n", ae.Code)
        fmt.Printf("Message: %s\n", ae.Message)
    } else {
        fmt.Printf("Network error: %v\n", err)
    }
}
```

## 各产品参数速查

| 文档 | Endpoint | 示例 Version |
| --- | --- | --- |
| 云服务器 ECS | `ecs.aliyuncs.com` | `2014-05-26` |
| 专有网络 VPC | `vpc.aliyuncs.com` | `2016-04-28` |
| 负载均衡 SLB | `slb.aliyuncs.com` | `2014-05-15` |
| 云数据库 RDS | `rds.aliyuncs.com` | `2014-08-15` |
| 云数据库 MongoDB | `mongodb.aliyuncs.com` | `2015-12-01` |
| 云数据库 Redis/Tair | `r-kvstore.aliyuncs.com` | `2015-01-01` |
| RocketMQ | `rocketmq.aliyuncs.com` | `2019-01-01` |
| 消息队列 Kafka | `kafka.aliyuncs.com` | `2019-01-01` |
| 专有云 DNS | `dns.aliyuncs.com` | `2015-01-09` |
| 云服务总线 CSB | `csb.aliyuncs.com` | `2017-11-18` |

## API 参考

### 创建 Client

```go
func NewClient(opts ...ClientOption) (*Client, error)
```

### ClientOption

| 选项 | 说明 | 环境变量替代 |
| --- | --- | --- |
| `WithEndpoint(s)` | API 服务地址（必填） | `APSARA_ENDPOINT` |
| `WithRegion(s)` | 地域 ID（必填） | `APSARA_REGION_ID` |
| `WithCredential(c)` | 访问凭证 | `APSARA_ACCESS_KEY_ID` / `APSARA_ACCESS_KEY_SECRET` / `APSARA_SECURITY_TOKEN` |
| `WithInsecureSkipVerify(b)` | 跳过 TLS 验证 | — |
| `WithHTTPClient(cl)` | 自定义 HTTP 客户端 | — |
| `WithOrganizationID(s)` | 组织 ID | — |
| `WithResourceGroupID(s)` | 资源集 ID | — |
| `WithInstanceID(s)` | 实例 ID | — |
| `WithCallerSource(s)` | 调用来源标识 | — |
| `WithLogger(l)` | 日志记录器 | — |
| `WithRetry(n)` | 最大重试次数 | — |
| `WithTimeout(d)` | 单次 HTTP 请求总超时 | — |

### RequestOption

| 选项 | 说明 |
| --- | --- |
| `WithMeta(m *ResponseMeta)` | 获取响应元数据（RequestId、状态码、Header、原始 Body） |

### 错误类型

```go
type ApsaraError struct {
    Action     string
    StatusCode int
    RequestID  string
    Code       string
    Message    string
    Err        error
}
```

## 设计参考

本 SDK 的设计参考了 AWS SDK v2 的关键模式：

- **函数选项模式**（Functional Options）：`NewClient(opts...)` 可扩展不破坏签名
- **凭证链**（Credential Chain）：环境变量 → 手动凭证
- **结构化错误**（Structured Error）：`ApsaraError` 类似 AWS 的 `smithy.RequestError`
- **指数退避重试**（Exponential Backoff + Jitter）：内置 `backoff()` 函数
- **响应元数据**（Response Metadata）：通过 `WithMeta` 选项注入

## 许可证

MIT
