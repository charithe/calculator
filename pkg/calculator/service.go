package calculator

import (
	"context"
	"io"

	"github.com/charithe/calculator/pkg/v1pb"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
)

// Service implements the RPC interface of the calculator
type Service struct {
	*health.Server
}

func NewService() *Service {
	return &Service{
		Server: health.NewServer(),
	}
}

func (s *Service) EvaluateStream(stream v1pb.Calculator_EvaluateStreamServer) error {
	rpn := &rpnEvaluator{}

	for {
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				// end of the client-side stream so calculate the result
				result, err := rpn.result()
				if err != nil {
					return grpc.Errorf(codes.InvalidArgument, err.Error())
				}

				if err := stream.SendAndClose(&v1pb.EvaluateStreamResponse{Result: result}); err != nil {
					zap.S().Errorw("Failed to send response", "error", err)
					return err
				}

				return nil
			}

			zap.S().Warnw("Failed to receive request from stream", "error", err)
			return err
		}

		switch v := req.Token.Token.(type) {
		case *v1pb.Token_Operand:
			if err := rpn.pushOperand(v.Operand.Value); err != nil {
				return grpc.Errorf(codes.ResourceExhausted, err.Error())
			}
		case *v1pb.Token_Operator:
			if err := rpn.pushOperator(v.Operator); err != nil {
				return grpc.Errorf(codes.InvalidArgument, err.Error())
			}
		}
	}
}

func (s *Service) EvaluateBatch(ctx context.Context, req *v1pb.EvaluateBatchRequest) (*v1pb.EvaluateBatchResponse, error) {
	// if the context has already expired, we can avoid unnecessary work
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rpn := &rpnEvaluator{}
	for _, t := range req.Tokens {
		switch v := t.Token.(type) {
		case *v1pb.Token_Operand:
			if err := rpn.pushOperand(v.Operand.Value); err != nil {
				return nil, grpc.Errorf(codes.ResourceExhausted, err.Error())
			}
		case *v1pb.Token_Operator:
			if err := rpn.pushOperator(v.Operator); err != nil {
				return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
			}
		}
	}

	result, err := rpn.result()
	if err != nil {
		return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
	}

	return &v1pb.EvaluateBatchResponse{Result: result}, nil
}
