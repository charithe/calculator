package calculator

import (
	"context"
	"net"
	"testing"

	"github.com/charithe/calculator/pkg/v1pb"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestCalculator(t *testing.T) {
	svc := NewService()
	addr, destroyFunc := startServer(t, svc)
	defer destroyFunc()

	client := createClient(t, addr)
	defer client.Close()

	testCases := []struct {
		name       string
		tokens     []string
		wantResult float64
		wantErr    bool
	}{
		{
			name:       "validTokens",
			tokens:     []string{"5", "8", " + ", " 3 ", "-", "2", "/", "5", "*"},
			wantResult: 25,
		},
		{
			name:    "invalidTokens",
			tokens:  []string{"5", "a", "+"},
			wantErr: true,
		},
		{
			name:    "invalidOrder1",
			tokens:  []string{"+"},
			wantErr: true,
		},
		{
			name:    "invalidOrder2",
			tokens:  []string{"5", "5", "5", "+"},
			wantErr: true,
		},
		{
			name:    "emptyTokens",
			tokens:  []string{},
			wantErr: true,
		},
	}

	t.Run("stream", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				tokChan := make(chan string)
				go func(tokens []string) {
					for _, tok := range tokens {
						tokChan <- tok
					}
					close(tokChan)
				}(tc.tokens)

				haveResult, err := client.EvaluateStream(tokChan)
				if tc.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, tc.wantResult, haveResult)
				}
			})
		}
	})

	t.Run("batch", func(t *testing.T) {
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				haveResult, err := client.EvaluateBatch(context.Background(), tc.tokens)
				if tc.wantErr {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
					require.Equal(t, tc.wantResult, haveResult)
				}
			})
		}
	})
}

func startServer(t *testing.T, service *Service) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := lis.Addr().String()
	srv := grpc.NewServer()
	v1pb.RegisterCalculatorServer(srv, service)

	go func() {
		if err := srv.Serve(lis); err != nil {
			panic(err)
		}
	}()

	destroyFunc := func() {
		srv.GracefulStop()
		lis.Close()
	}

	return addr, destroyFunc
}

func createClient(t *testing.T, addr string) *Client {
	t.Helper()

	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}

	return NewClient(conn)
}
