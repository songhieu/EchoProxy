package grpc

import (
	"context"
	"errors"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"echoproxy/ingest-api/internal/domain"
	"echoproxy/ingest-api/internal/usecase"
	"echoproxy/pkg/event"
)

// Server bridges the generated EventIngestServer to the use case.
type Server struct {
	event.UnimplementedEventIngestServer
	uc *usecase.Ingest
}

func NewServer(uc *usecase.Ingest) *Server { return &Server{uc: uc} }

func (s *Server) Ingest(ctx context.Context, req *event.IngestRequest) (*event.IngestResponse, error) {
	apiKey, err := apiKeyFromMD(ctx)
	if err != nil {
		return nil, err
	}
	res, err := s.uc.Execute(ctx, apiKey, req.GetEvents())
	if err := mapErr(err); err != nil {
		return nil, err
	}
	return &event.IngestResponse{Accepted: res.Accepted, Rejected: res.Rejected, Reason: res.Reason}, nil
}

func (s *Server) IngestStream(stream event.EventIngest_IngestStreamServer) error {
	apiKey, err := apiKeyFromMD(stream.Context())
	if err != nil {
		return err
	}
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		res, err := s.uc.Execute(stream.Context(), apiKey, req.GetEvents())
		if mapped := mapErr(err); mapped != nil {
			return mapped
		}
		if err := stream.Send(&event.IngestResponse{
			Accepted: res.Accepted, Rejected: res.Rejected, Reason: res.Reason,
		}); err != nil {
			return err
		}
	}
}

func apiKeyFromMD(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing metadata")
	}
	if v := md.Get("x-echo-key"); len(v) > 0 {
		return v[0], nil
	}
	return "", status.Error(codes.Unauthenticated, "x-echo-key metadata required")
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, domain.ErrAPIKeyNotFound):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, domain.ErrAPIKeyRevoked):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, domain.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
