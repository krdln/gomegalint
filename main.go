package main

import (
	// "bytes"
	"fmt"
	"go/ast"
	// "go/printer"
	// "go/token"
	// "go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() {
	singlechecker.Main(Analyzer)
}

var Analyzer = &analysis.Analyzer{
	Name: "gomegalint",
	Doc:  "reports non-idiomatic usage of gomega matchers",
	Run:  run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	for _, file := range pass.Files {
		ast.Inspect(file, func(n ast.Node) bool {
			ass := getAssertion(n)
			if ass == nil {
				// FIXME perhaps don't always recurse?
				return true
			}

			checkStyle(*ass, pass)

			// FIXME what should we return here? Can assertions be nested?
			return true
		})
	}

	return nil, nil
}

// assertion describes a `Ω(X).Should(Y)`-like call
type assertion struct {
	*ast.CallExpr            // whole
	Omega         *ast.Ident // Ω part
	Subject       ast.Expr   // X part
	Should        *ast.Ident // Should part
	Matcher       ast.Expr   // Y part
	Negated       bool       // whether the matcher is negated (eg. when using `ShouldNot`)
}

const Omega = "Ω"
const Expect = "Expect"
const Should = "Should"
const ShouldNot = "ShouldNot"
const To = "To"
const ToNot = "ToNot"
const NotTo = "NotTo"

type Style int

const (
	ShouldStyle Style = iota
	ExpectStyle
)

func checkStyle(ass assertion, pass *analysis.Pass) {
	if getStyle(ass.Omega.Name) != getStyle(ass.Should.Name) {
		d := analysis.Diagnostic{
			Pos: ass.Pos(),
			End: ass.End(),
			Message: fmt.Sprintf(
				"incosistent assertion style (%s + %s)",
				ass.Omega.Name, ass.Should.Name,
			),
			SuggestedFixes: []analysis.SuggestedFix{{
				Message: "",
				TextEdits: []analysis.TextEdit{{
					Pos:     ass.Should.Pos(),
					End:     ass.Should.End(),
					NewText: []byte(renderInStyle(getStyle(ass.Omega.Name), ass.Negated)),
				}},
			}},
		}
		pass.Report(d)
	}
}

func getAssertion(n ast.Node) *assertion {
	call, ok := n.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil
	}

	shouldGetter, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	omegaCall, ok := shouldGetter.X.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil
	}

	omega, ok := omegaCall.Fun.(*ast.Ident)
	if !ok {
		return nil
	}

	switch omega.Name {
	case Omega, Expect:
		break
	default:
		return nil
	}

	negated := false
	switch shouldGetter.Sel.Name {
	case Should, To:
		break
	case ShouldNot, ToNot, NotTo:
		negated = true
		break
	default:
		return nil
	}

	return &assertion{
		CallExpr: call,
		Omega:    omega,
		Subject:  omegaCall.Args[0],
		Should:   shouldGetter.Sel,
		Negated:  negated,
		Matcher:  call.Args[0],
	}
}

func getStyle(s string) Style {
	switch s {
	case Omega, Should, ShouldNot:
		return ShouldStyle
	case Expect, To, ToNot, NotTo:
		return ExpectStyle
	default:
		panic("foo")
	}
}

func renderInStyle(style Style, negated bool) string {
	switch style {
	case ShouldStyle:
		if negated {
			return ShouldNot
		} else {
			return Should
		}
	case ExpectStyle:
		if negated {
			return NotTo
		} else {
			return To
		}
	default:
		panic("bar")
	}
}
