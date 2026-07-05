package main

import (
	"context"
	"log"
	"net"
	"os"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	proto "connect-to-mongodb/grpc-analysis/proto"
	appanalysis "connect-to-mongodb/internal/analysis"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println(".env not loaded, using environment variables")
	}

	repo, err := appanalysis.NewRepository(context.Background())
	if err != nil {
		log.Fatalf("failed to connect to MongoDB: %v", err)
	}
	defer repo.Close(context.Background())

	service := appanalysis.NewService(repo)
	grpcServer := grpc.NewServer()
	proto.RegisterAnalysisServiceServer(grpcServer, appanalysis.NewGRPCServer(service))
	reflection.Register(grpcServer)

	port := os.Getenv("GRPC_PORT")
	if port == "" {
		port = "50051"
	}

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", port, err)
	}

	log.Printf("gRPC analysis service listening on :%s", port)
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("gRPC server stopped: %v", err)
	}
}
