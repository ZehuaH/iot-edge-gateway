## IoT gRPC 边缘网关示例

这是一个使用 Go 编写的 **简单 gRPC IoT 边缘网关代码框架**，包含：

- **Edge Gateway gRPC 服务端**
- **基础 Telemetry（遥测）上报接口**
- **HealthCheck 健康检查接口**
- **简单 gRPC 压测工具（并发压测 Telemetry 接口）**

### 目录结构

- **`go.mod`**: Go 模块定义
- **`proto/gateway.proto`**: gRPC 接口定义
- **`cmd/gateway/main.go`**: 网关 gRPC 服务启动入口
- **`internal/gateway/server.go`**: 网关服务实现
- **`cmd/bench/main.go`**: 简单 gRPC 压测工具

### 1. 安装依赖

在项目根目录执行：

```bash
go mod tidy
```

### 2. 生成 gRPC 代码

需要先安装 `protoc` 和 `protoc-gen-go`、`protoc-gen-go-grpc` 插件。

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

将 `GOPATH/bin` 加入 `PATH` 后，在项目根目录执行：

```bash
protoc ^
  --go_out=. --go_opt=paths=source_relative ^
  --go-grpc_out=. --go-grpc_opt=paths=source_relative ^
  proto/gateway.proto
```

生成的代码会在 `pkg/gatewaypb` 下。

### 3. 启动 gRPC 网关服务

```bash
go run ./cmd/gateway
```

默认监听 `:50051`，可以通过环境变量或命令行参数（后续可扩展）修改。

### 4. 使用压测工具进行 gRPC 压测

在另一个终端中运行：

```bash
go run ./cmd/bench ^
  -target 127.0.0.1:50051 ^
  -concurrency 50 ^
  -total 5000
```

参数说明：

- **`-target`**: gRPC 服务地址（host:port）
- **`-concurrency`**: 并发协程数
- **`-total`**: 总请求数（所有协程合计）

输出会包含：

- 成功/失败请求数
- QPS（Requests Per Second）
- 平均延迟、最大延迟

### 5. 后续可扩展方向

- **设备认证/注册**（Token、证书等）
- **流式 RPC**（例如 Command 下发、双向流）
- **持久化存储**（InfluxDB / TimeSeries / Kafka 等）
- **多协议接入**（MQTT/HTTP → gRPC 汇聚）

本仓库目前提供的是一个 **最小可运行的骨架**，方便在此基础上扩展真实业务逻辑。

