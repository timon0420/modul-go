package analysis

import (
	"context"

	proto "connect-to-mongodb/grpc-analysis/proto"
)

type GRPCServer struct {
	proto.UnimplementedAnalysisServiceServer
	service *Service
}

func NewGRPCServer(service *Service) *GRPCServer {
	return &GRPCServer{service: service}
}

func (s *GRPCServer) AnalyzeUser(ctx context.Context, req *proto.AnalyzeRequest) (*proto.AnalysisResponse, error) {
	report, err := s.service.AnalyzeUser(ctx, req.GetLogin())
	if err != nil {
		return nil, err
	}

	return &proto.AnalysisResponse{
		Login:               report.Login,
		Date:                report.Date,
		TotalDuration:       int32(report.TotalDuration),
		GlobalLimitExceeded: report.GlobalLimitExceeded,
		ExceededLimits:      report.ExceededLimits,
	}, nil
}
