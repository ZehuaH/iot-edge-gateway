# 优化版 IoT gRPC 边缘网关

这是基于基础版 `D:\Goproj\iot` 的**优化版实现**，用于 A/B 性能对比。

## 优化点

| 优化方向 | 基础版 | 优化版 |
|----------|--------|--------|
| 数据结构 | `map<string, double>` + 字符串 key | `repeated Metric{id, value}` 紧凑结构 |
| 上报方式 | 每条样本一次 unary RPC | 支持批量上报 + 流式 RPC |
| 内存管理 | 每次 `make(map...)` 新分配 | `sync.Pool` 对象池复用 |
| 请求调度 | 所有请求共享同一路径 | 高/低优先级队列分离 |
| 数值精度 | `double` (8 字节) | `float` (4 字节)，省一半空间 |

## 目录结构

```
optimized/
├── proto/
│   └── gateway_optimized.proto   # 优化版接口定义
├── internal/
│   └── gateway/
│       ├── server.go             # 优化版 server（含优先级队列）
│       └── pool.go               # sync.Pool 对象池
├── cmd/
│   ├── gateway/main.go           # 网关启动入口（默认端口 50052）
│   └── bench/
│       ├── edge_bench/main.go    # 背景流量生成工具
│       └── edge_metrics/main.go  # 单设备采样工具（生成 CSV）
├── plot_compare.py               # 对比画图脚本
└── README.md                     # 本文件
```

## 快速开始

### 1. 生成 proto 代码

在项目根目录执行：

```powershell
cd D:\Goproj\iot

protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative optimized/proto/gateway_optimized.proto
```

### 2. 启动优化版网关

```powershell
go run ./optimized/cmd/gateway
```

默认参数：
- 端口：`50052`（基础版是 `50051`）
- GPU 模拟延迟：`50ms`
- 高优先级队列容量：`100`
- 低优先级队列容量：`1000`

可通过参数调整：

```powershell
go run ./optimized/cmd/gateway `
  -port 50052 `
  -gpu-latency-ms 30 `
  -high-queue-size 200 `
  -low-queue-size 2000
```

### 3. 运行背景流量（edge_bench）

**单次上报模式（对照基础版）**：

```powershell
go run ./optimized/cmd/bench/edge_bench `
  -target 127.0.0.1:50052 `
  -edges 20 `
  -devices-per-edge 1 `
  -vib-per-msg 2000 `
  -interval-ms 20
```

**批量上报模式**（多条样本合并一次发送）：

```powershell
go run ./optimized/cmd/bench/edge_bench `
  -target 127.0.0.1:50052 `
  -edges 20 `
  -batch `
  -batch-size 5
```

**流式上报模式**（长连接持续写入）：

```powershell
go run ./optimized/cmd/bench/edge_bench `
  -target 127.0.0.1:50052 `
  -edges 20 `
  -stream
```

**列车实时流量（高优先级）**：

```powershell
go run ./optimized/cmd/bench/edge_bench `
  -target 127.0.0.1:50052 `
  -edges 1 `
  -priority 1
```

### 4. 采样单设备并生成 CSV（edge_metrics）

```powershell
go run ./optimized/cmd/bench/edge_metrics `
  -target 127.0.0.1:50052 `
  -edge 0 `
  -device 0 `
  -vib-per-msg 2000 `
  -interval-ms 20 `
  -duration-sec 60 `
  -out optimized_edge
```

结束后生成：
- `optimized_edge_latency.csv`：每次请求的延迟
- `optimized_edge_qps.csv`：每秒请求数

支持批量模式：

```powershell
go run ./optimized/cmd/bench/edge_metrics `
  -target 127.0.0.1:50052 `
  -batch `
  -batch-size 5 `
  -duration-sec 60 `
  -out optimized_batch
```

## A/B 对比测试

### 同时启动两个网关

终端 1（基础版，端口 50051）：

```powershell
go run ./cmd/gateway
```

终端 2（优化版，端口 50052）：

```powershell
go run ./optimized/cmd/gateway
```

### 分别采样

终端 3（基础版采样）：

```powershell
go run ./cmd/bench/edge_metrics `
  -target 127.0.0.1:50051 `
  -duration-sec 60 `
  -out baseline_edge
```

终端 4（优化版采样）：

```powershell
go run ./optimized/cmd/bench/edge_metrics `
  -target 127.0.0.1:50052 `
  -duration-sec 60 `
  -out optimized_edge
```

### 对比画图

```bash
python optimized/plot_compare.py `
  --baseline-latency baseline_edge_latency.csv `
  --baseline-qps baseline_edge_qps.csv `
  --optimized-latency optimized_edge_latency.csv `
  --optimized-qps optimized_edge_qps.csv
```

## 优先级队列说明

优化版 server 内部有两个队列：

```
请求进入
   │
   ├─ priority=1 ──► 高优先级队列（容量小，4 个 worker 快速处理）
   │                  └─ 用于：列车经过时的实时流量
   │
   └─ priority=0 ──► 低优先级队列（容量大，8 个 worker 批量处理）
                      └─ 用于：平时背景数据、历史补传
```

当高优先级队列满时，会降级到低优先级队列；当两个队列都满时，返回 `REJECTED`。

## 参数说明

### gateway 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-port` | 50052 | gRPC 监听端口 |
| `-gpu-latency-ms` | 50 | 模拟 GPU 推理延迟（毫秒） |
| `-high-queue-size` | 100 | 高优先级队列容量 |
| `-low-queue-size` | 1000 | 低优先级队列容量 |

### edge_bench 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-target` | 127.0.0.1:50052 | gRPC 服务地址 |
| `-edges` | 20 | edge 设备数量 |
| `-devices-per-edge` | 1 | 每个 edge 下的设备数 |
| `-vib-per-msg` | 2000 | 每条消息的振动采样点数 |
| `-interval-ms` | 20 | 发送间隔（毫秒） |
| `-batch` | false | 启用批量上报模式 |
| `-batch-size` | 5 | 批量大小（条数） |
| `-stream` | false | 启用流式上报模式 |
| `-priority` | 0 | 优先级（0=背景，1=实时） |

### edge_metrics 参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `-target` | 127.0.0.1:50052 | gRPC 服务地址 |
| `-edge` | 0 | edge 编号（0-based） |
| `-device` | 0 | 设备编号（0-based） |
| `-duration-sec` | 60 | 测试时长（秒） |
| `-out` | optimized_edge_metrics | 输出文件前缀 |
| `-batch` | false | 启用批量上报模式 |
| `-batch-size` | 5 | 批量大小 |
| `-priority` | 0 | 优先级 |

## 预期优化效果

在相同负载下，优化版相比基础版应该能观察到：

- **QPS 提升**：紧凑结构减少序列化开销，批量/流式减少 RPC 次数
- **平均延迟降低**：对象池减少 GC 停顿
- **最大延迟（p99）收窄**：优先级队列保证实时流量不被背景流量拖慢
- **内存占用更稳定**：对象复用减少分配抖动
