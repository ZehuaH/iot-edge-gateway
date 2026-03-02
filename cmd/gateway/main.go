package main

import (
	"flag"
	"fmt"
	"log"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"iot/internal/gateway"
	gatewaypb "iot/proto"
)

func main() {
	var (
		host = flag.String("host", "0.0.0.0", "gRPC 服务监听地址")
		port = flag.Int("port", 50051, "gRPC 服务监听端口")
	)
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", *host, *port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", addr, err)
	}

	grpcServer := grpc.NewServer()

	// 注册网关服务实现
	s := gateway.NewServer()
	gatewaypb.RegisterEdgeGatewayServiceServer(grpcServer, s)

	// 注册反射服务，便于调试（例如使用 grpcurl）
	reflection.Register(grpcServer)

	log.Printf("EdgeGateway gRPC server listening on %s", addr)

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

