package main

import (
	"fmt"
	"go/ast"
	"go/types"

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

			emittedShouldFix := checkNilnessAssertions(*ass, pass)
			checkStyle(*ass, pass, !emittedShouldFix)

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

func checkStyle(ass assertion, pass *analysis.Pass, emitFixes bool) {
	if getStyle(ass.Omega.Name) == getStyle(ass.Should.Name) {
		return
	}

	d := analysis.Diagnostic{
		Pos: ass.Pos(),
		End: ass.End(),
		Message: fmt.Sprintf(
			"inconsistent assertion style (%s + %s)",
			ass.Omega.Name, ass.Should.Name,
		),
	}

	if emitFixes {
		fixedShould := renderInStyle(getStyle(ass.Omega.Name), ass.Negated)
		d.SuggestedFixes = []analysis.SuggestedFix{{
			Message: fmt.Sprintf("change %s to %s", ass.Should.Name, fixedShould),
			TextEdits: []analysis.TextEdit{{
				Pos:     ass.Should.Pos(),
				End:     ass.Should.End(),
				NewText: []byte(fixedShould),
			}},
		}}
	}

	pass.Report(d)
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

// checkNilnessAssertions checks which of IsNil/HaveOccurred/Succeed matchers
// is the most appropriate and returns, whether a TextEdit for 'Should' part of
// an assertion was emitted (because of possible need of inverting the
// condition)
func checkNilnessAssertions(ass assertion, pass *analysis.Pass) (emittedShouldFix bool) {
	matcherIdent, matcher := getKnownMatcher(ass)
	if matcher == UnknownMatcher {
		return false
	}

	var expectedMatcher KnownMatcher
	if isErrorExpr(ass.Subject, pass.TypesInfo) {
		if _, isCall := ass.Subject.(*ast.CallExpr); isCall {
			expectedMatcher = Succeed
		} else {
			expectedMatcher = HaveOccurred
		}
	} else {
		expectedMatcher = IsNil
	}

	if matcher == expectedMatcher {
		return false
	}

	d := analysis.Diagnostic{
		Pos: ass.Pos(),
		End: ass.End(),
		Message: fmt.Sprintf(
			"unidiomatic matcher: consider using %s instead of %s in this assertion",
			expectedMatcher, matcher,
		),
		SuggestedFixes: []analysis.SuggestedFix{{
			Message: fmt.Sprintf("change matcher to %s", expectedMatcher),
			TextEdits: []analysis.TextEdit{{
				Pos:     matcherIdent.Pos(),
				End:     matcherIdent.End(),
				NewText: []byte(expectedMatcher),
			}},
		}},
	}

	needsInverting := matchesNil(matcher) != matchesNil(expectedMatcher)
	if needsInverting {
		d.SuggestedFixes[0].TextEdits = append(d.SuggestedFixes[0].TextEdits, analysis.TextEdit{
			Pos:     ass.Should.Pos(),
			End:     ass.Should.End(),
			NewText: []byte(renderInStyle(getStyle(ass.Omega.Name), ass.Negated != needsInverting)),
		})
		d.SuggestedFixes[0].Message += " and invert the assertion"
	}

	pass.Report(d)

	return needsInverting
}

type KnownMatcher string

const (
	UnknownMatcher KnownMatcher = ""
	IsNil          KnownMatcher = "BeNil"
	HaveOccurred   KnownMatcher = "HaveOccurred"
	Succeed        KnownMatcher = "Succeed"
)

func matchesNil(m KnownMatcher) bool { return m != HaveOccurred }

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

func getKnownMatcher(ass assertion) (*ast.Ident, KnownMatcher) {
	call, ok := ass.Matcher.(*ast.CallExpr)
	if !ok || len(call.Args) != 0 {
		return nil, UnknownMatcher
	}

	matcherIdent, ok := call.Fun.(*ast.Ident)
	if !ok {
		return nil, UnknownMatcher
	}

	knownMatcher := KnownMatcher(matcherIdent.Name)
	switch knownMatcher {
	case IsNil, HaveOccurred, Succeed:
		return matcherIdent, knownMatcher
	default:
		return nil, UnknownMatcher
	}
}

// isErrorExpr returns whether expr's type is an error or something that implements it
func isErrorExpr(e ast.Expr, info *types.Info) bool {
	t := info.Types[e].Type
	return types.Implements(t, errorInterface)
}

var errorInterface = types.Universe.Lookup("error").Type().Underlying().(*types.Interface)
