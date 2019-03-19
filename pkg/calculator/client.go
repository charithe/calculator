package calculator

import (
	"context"
	"strconv"
	"strings"

	"github.com/charithe/calculator/pkg/v1pb"
	"google.golang.org/grpc"
)

// Client implements the RPC client for the Calculator service
type Client struct {
	conn   *grpc.ClientConn
	client v1pb.CalculatorClient
}

func NewClient(conn *grpc.ClientConn) *Client {
	return &Client{
		conn:   conn,
		client: v1pb.NewCalculatorClient(conn),
	}
}

func (c *Client) EvaluateStream(tokens <-chan string) (float64, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := c.client.EvaluateStream(ctx)
	if err != nil {
		return 0, err
	}

	for tokenStr := range tokens {
		tok, err := parseToken(tokenStr)
		if err != nil {
			return 0, err
		}

		if err := stream.Send(&v1pb.EvaluateStreamRequest{Token: tok}); err != nil {
			return 0, err
		}
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return 0, err
	}

	return resp.Result, nil
}

func (c *Client) EvaluateBatch(ctx context.Context, tokenStrs []string) (float64, error) {
	tokens := make([]*v1pb.Token, len(tokenStrs))
	for i, tokStr := range tokenStrs {
		tok, err := parseToken(tokStr)
		if err != nil {
			return 0, err
		}

		tokens[i] = tok
	}

	resp, err := c.client.EvaluateBatch(ctx, &v1pb.EvaluateBatchRequest{Tokens: tokens})
	if err != nil {
		return 0, err
	}

	return resp.Result, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func parseToken(tokenStr string) (*v1pb.Token, error) {
	tokStr := strings.TrimSpace(tokenStr)
	switch tokStr {
	case "+":
		return &v1pb.Token{Token: &v1pb.Token_Operator{Operator: v1pb.ADD}}, nil
	case "-":
		return &v1pb.Token{Token: &v1pb.Token_Operator{Operator: v1pb.SUBTRACT}}, nil
	case "*":
		return &v1pb.Token{Token: &v1pb.Token_Operator{Operator: v1pb.MULTIPLY}}, nil
	case "/":
		return &v1pb.Token{Token: &v1pb.Token_Operator{Operator: v1pb.DIVIDE}}, nil
	default:
		v, err := strconv.ParseFloat(tokStr, 64)
		if err != nil {
			return nil, err
		}
		return &v1pb.Token{Token: &v1pb.Token_Operand{Operand: &v1pb.Operand{Value: v}}}, nil
	}
}
