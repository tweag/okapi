package okapi

import (
	"github.com/bazelbuild/bazel-gazelle/rule"
)

func ppxExecutable(name string, deps []string) *rule.Rule {
	r := rule.NewRule("ppx_executable", ppxName(name))
	r.SetAttr("deps_opam", deps)
	r.SetAttr("main", "@obazl_rules_ocaml//dsl:ppx_driver")
	return r
}

type PpxKind interface {
	exe(name string) []RuleResult
	depsOpam() []string
	isPpx() bool
}

type PpxTransitive struct{}
type PpxDirect struct{ deps []string }
type NoPpx struct{}

func (PpxTransitive) exe(string) []RuleResult { return nil }
func (ppx PpxDirect) exe(slug string) []RuleResult {
	return []RuleResult{{ppxExecutable(slug, ppx.deps), nil}}
}
func (NoPpx) exe(string) []RuleResult { return nil }

func (PpxTransitive) depsOpam() []string { return nil }
func (ppx PpxDirect) depsOpam() []string { return ppx.deps }
func (NoPpx) depsOpam() []string         { return nil }

func (PpxTransitive) isPpx() bool { return true }
func (PpxDirect) isPpx() bool     { return true }
func (NoPpx) isPpx() bool         { return false }
