package tools

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"math"
	"strconv"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// CalculatorInput defines the input parameters for the calculator tool.
type CalculatorInput struct {
	Expression string `json:"expression" jsonschema:"description=The mathematical expression to evaluate. Supports basic arithmetic (+, -, *, /) and parentheses. Examples: '1+2', '(3+4)*5', '10/2'"`
}

// CalculatorOutput defines the output of the calculator tool.
type CalculatorOutput struct {
	Result float64 `json:"result"`
	Error  string  `json:"error,omitempty"`
}

// NewCalculatorTool creates a new calculator tool.
func NewCalculatorTool(ctx context.Context) (tool.InvokableTool, error) {
	return utils.InferTool(
		ToolIDCalculator,
		"A calculator that evaluates mathematical expressions. Supports basic arithmetic operations (+, -, *, /) and parentheses.",
		func(ctx context.Context, input *CalculatorInput) (*CalculatorOutput, error) {
			result, err := evaluateExpression(input.Expression)
			if err != nil {
				return &CalculatorOutput{
					Error: err.Error(),
				}, nil
			}
			return &CalculatorOutput{
				Result: result,
			}, nil
		},
	)
}

// evaluateExpression evaluates a simple mathematical expression.
// This is a safe implementation that only allows basic arithmetic.
func evaluateExpression(expr string) (float64, error) {
	// Parse the expression as a Go expression (safe subset)
	node, err := parser.ParseExpr(expr)
	if err != nil {
		return 0, fmt.Errorf("invalid expression: %w", err)
	}

	return evalNode(node)
}

func evalNode(node ast.Expr) (float64, error) {
	switch n := node.(type) {
	case *ast.BasicLit:
		// Number literal
		if n.Kind == token.INT || n.Kind == token.FLOAT {
			return strconv.ParseFloat(n.Value, 64)
		}
		return 0, fmt.Errorf("unsupported literal type: %v", n.Kind)

	case *ast.BinaryExpr:
		// Binary operation
		left, err := evalNode(n.X)
		if err != nil {
			return 0, err
		}
		right, err := evalNode(n.Y)
		if err != nil {
			return 0, err
		}

		switch n.Op {
		case token.ADD:
			return left + right, nil
		case token.SUB:
			return left - right, nil
		case token.MUL:
			return left * right, nil
		case token.QUO:
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			return left / right, nil
		case token.REM:
			if right == 0 {
				return 0, fmt.Errorf("modulo by zero")
			}
			return math.Mod(left, right), nil
		default:
			return 0, fmt.Errorf("unsupported operator: %v", n.Op)
		}

	case *ast.ParenExpr:
		// Parenthesized expression
		return evalNode(n.X)

	case *ast.UnaryExpr:
		// Unary operation (e.g., -5)
		x, err := evalNode(n.X)
		if err != nil {
			return 0, err
		}
		switch n.Op {
		case token.SUB:
			return -x, nil
		case token.ADD:
			return x, nil
		default:
			return 0, fmt.Errorf("unsupported unary operator: %v", n.Op)
		}

	default:
		return 0, fmt.Errorf("unsupported expression type: %T", node)
	}
}
