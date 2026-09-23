//go:build ruleguard

package gorules

import (
	"github.com/quasilyte/go-ruleguard/dsl"
)

// unwrappedErr used for linting, flags a bare `return err` / `return _, err`
func unwrappedErr(m dsl.Matcher) {
	m.Match(`return $err`, `return $_, $err`).
		Where(m["err"].Type.Is(`error`) &&
			m["err"].Node.Is(`Ident`)).
		Report("please either wrap in fmt.Errorf, or add: '//nolint // <reason why we don't need to wrap error>'; for details, see: AGENTS.md")
}
