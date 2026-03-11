package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"iot/optimized/internal/gateway"
	optimizedpb "iot/optimized/proto"
)

func main() {
	var (
		host           = flag.String("host", "0.0.0.0", "gRPC 服务监听地址")
		port           = flag.Int("port", 50052, "gRPC 服务监听端口（优化版默认 50052）")
		gpuLatencyMs   = flag.Int64("gpu-latency-ms", 50, "模拟 GPU 推理延迟（毫秒）")
		highQueueSize  = flag.Int("high-queue-size", 100, "高优先级队列容量")
		lowQueueSize   = flag.Int("low-queue-size", 1000, "低优先级队列容量")
	)
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", *host, *port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", addr, err)
	}

	grpcServer := grpc.NewServer()

	// 创建优化版网关服务
	s := gateway.NewServer(*gpuLatencyMs, *highQueueSize, *lowQueueSize)
	optimizedpb.RegisterOptimizedEdgeGatewayServiceServer(grpcServer, s)

	// 注册反射服务
	reflection.Register(grpcServer)

	log.Printf("Optimized EdgeGateway gRPC server listening on %s", addr)
	log.Printf("Config: gpuLatencyMs=%d, highQueueSize=%d, lowQueueSize=%d",
		*gpuLatencyMs, *highQueueSize, *lowQueueSize)

	// 优雅退出
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		s.Stop()
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
