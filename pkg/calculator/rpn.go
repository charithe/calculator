package calculator

import (
	"errors"
	"fmt"

	"github.com/charithe/calculator/pkg/v1pb"
)

const stackSize = 16

// rpnEvaluator implements a RPN expression evaluator.
// This is not thread-safe and should only be accessed by a single goroutine.
type rpnEvaluator struct {
	stack [stackSize]float64
	ptr   int
}

func (r *rpnEvaluator) pushOperand(v float64) error {
	if r.ptr >= stackSize-1 {
		return errors.New("stack full")
	}

	r.stack[r.ptr] = v
	r.ptr++
	return nil
}

func (r *rpnEvaluator) pop() float64 {
	r.ptr--
	return r.stack[r.ptr]
}

func (r *rpnEvaluator) pushOperator(op v1pb.Operator) error {
	if r.ptr < 2 {
		return errors.New("not enough operands")
	}

	v2 := r.pop()
	v1 := r.pop()
	var result float64

	switch op {
	case v1pb.ADD:
		result = v1 + v2
	case v1pb.SUBTRACT:
		result = v1 - v2
	case v1pb.MULTIPLY:
		result = v1 * v2
	case v1pb.DIVIDE:
		result = v1 / v2
	default:
		return fmt.Errorf("unimplemented operator: %s", op)
	}

	return r.pushOperand(result)
}

func (r *rpnEvaluator) result() (float64, error) {
	if r.ptr != 1 {
		return 0, errors.New("incomplete expression: unused operands still in stack")
	}

	return r.pop(), nil
}
