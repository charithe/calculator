package calculator

import (
	"testing"

	"github.com/charithe/calculator/pkg/v1pb"
	"github.com/stretchr/testify/require"
)

func TestRPNEvaluator(t *testing.T) {
	t.Run("pushOperand", func(t *testing.T) {
		rpn := &rpnEvaluator{}
		for i := 0; i < stackSize-1; i++ {
			require.NoError(t, rpn.pushOperand(24))
		}

		// stack is full so the next push should return an error
		require.Error(t, rpn.pushOperand(24))
	})

	t.Run("pushOperator", func(t *testing.T) {
		rpn := &rpnEvaluator{}
		rpn.pushOperand(10)
		rpn.pushOperand(20)

		require.NoError(t, rpn.pushOperator(v1pb.ADD))
		// only one operand in stack so the next operator push should fail
		require.Error(t, rpn.pushOperator(v1pb.SUBTRACT))
	})

	t.Run("resultCalculation", func(t *testing.T) {
		testCases := []struct {
			name       string
			operands   []float64
			operators  []v1pb.Operator
			wantResult float64
		}{
			{
				name:       "add",
				operands:   []float64{10, 2},
				operators:  []v1pb.Operator{v1pb.ADD},
				wantResult: 12,
			},
			{
				name:       "subtract",
				operands:   []float64{10, 2},
				operators:  []v1pb.Operator{v1pb.SUBTRACT},
				wantResult: 8,
			},
			{
				name:       "multiply",
				operands:   []float64{10, 2},
				operators:  []v1pb.Operator{v1pb.MULTIPLY},
				wantResult: 20,
			},
			{
				name:       "divide",
				operands:   []float64{10, 2},
				operators:  []v1pb.Operator{v1pb.DIVIDE},
				wantResult: 5,
			},
			{
				name:       "multiply_subract_add",
				operands:   []float64{10, 2, 5, 9},
				operators:  []v1pb.Operator{v1pb.ADD, v1pb.SUBTRACT, v1pb.MULTIPLY},
				wantResult: -120,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				rpn := &rpnEvaluator{}
				for _, v := range tc.operands {
					require.NoError(t, rpn.pushOperand(v))
				}

				for _, op := range tc.operators {
					require.NoError(t, rpn.pushOperator(op))
				}

				haveResult, err := rpn.result()
				require.NoError(t, err)
				require.Equal(t, tc.wantResult, haveResult)

			})
		}
	})
}
