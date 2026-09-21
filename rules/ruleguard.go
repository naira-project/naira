//go:build ruleguard

package gorules

import (
	"github.com/quasilyte/go-ruleguard/dsl"
)

// unwrappedErr flags a bare `return err` / `return _, err` where the
// identifier is literally named "err"
func unwrappedErr(m dsl.Matcher) {
	m.Match(`return $err`, `return $_, $err`).
		Where(m["err"].Type.Is(`error`) &&
			m["err"].Node.Is(`Ident`) &&
			m["err"].Text == "err").
		Report("please either wrap in fmt.Errorf, or add: '//nolint:gocritic // <reason why we don't need to wrap error>'; for details, see: AGENTS.md")
}
