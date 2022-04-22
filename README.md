# gomegalint

Linter for [gomega][gomega] assertions.

1. Checks whether you're not mixing [Ω-notation with Expect-notation][notations] (which compiles, but looks weird).
2. Recommends [most appropriate matcher for checking nilness][errors] (`BeNil`/`Succeed`/`HaveOccurred`), depending on context.

Uses standard linter interface based on golang.org/x/tools/go/analysis.

## Example

```console
$ gomegalint -c 0 github.com/xxx/yyy/pkg/...
…/zzz.go:25:3: unidiomatic matcher: consider using HaveOccurred instead of BeNil in this assertion
25			Expect(err).Should(BeNil())
…/zzz.go:25:3: inconsistent assertion style (Expect + Should)
25			Expect(err).Should(BeNil())
```

All warnings are auto-fixable with the `-fix` flag!

```console
$ gomegalint -fix github.com/xxx/yyy/pkg/...
```

This applies the following suggestion:

```diff
-               Expect(err).Should(BeNil())
+               Expect(err).NotTo(HaveOccurred())
```

## Installation

```
go install github.com/krdln/gomegalint@latest
```

[gomega]: https://onsi.github.io/gomega/
[notations]: https://onsi.github.io/gomega/#making-assertions
[errors]: https://onsi.github.io/gomega/#handling-errors
