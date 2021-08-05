package okapi

import (
	"fmt"
	"log"

	"github.com/bazelbuild/bazel-gazelle/config"
	"github.com/bazelbuild/bazel-gazelle/label"
	"github.com/bazelbuild/bazel-gazelle/resolve"
	"github.com/bazelbuild/bazel-gazelle/rule"
)

type ResolvedLocal struct { label label.Label }
type ResolvedOpam struct {}

func importSpec(name string) resolve.ImportSpec { return resolve.ImportSpec{Lang: okapiName, Imp: name} }

func findImport(c *config.Config, ix *resolve.RuleIndex, name string) []resolve.FindResult {
  return ix.FindRulesByImportWithConfig(c, importSpec(name), okapiName)
}

func resolveDep(c *config.Config, ix *resolve.RuleIndex, dep string) interface{} {
  results := findImport(c, ix, dep)
  if len(results) == 0 {
    return ResolvedOpam{}
  } else if len(results) == 1 {
    r := results[0]
    return ResolvedLocal{r.Label}
  } else {
    log.Fatalf("Multiple libraries matched the depspec `%s`: %#v", dep, results)
    return nil
  }
}

func libraryDeps(
  c *config.Config,
  ix *resolve.RuleIndex,
  imports interface{},
  r *rule.Rule,
) {
  findDep := func (dep string) interface{} { return resolveDep(c, ix, dep) }
  var locals []string
  var opams []string
  if deps, isStrings := imports.([]string); isStrings {
    for _, dep := range deps {
      resolved := findDep(dep)
      if local, isLocal := resolved.(ResolvedLocal); isLocal {
        locals = append(locals, local.label.String())
      } else if _, isOpam := resolved.(ResolvedOpam); isOpam {
        opams = append(opams, dep)
      }
    }
    if len(locals) > 0 { r.SetAttr("deps", append(r.AttrStrings("deps"), locals...)) }
    if len(opams) > 0 { r.SetAttr("deps_opam", opams) }
  } else {
    log.Fatalf("Invalid type for imports of source file %s: %#v", r.Name(), imports)
  }
}

func implementationDeps(c *config.Config, ix *resolve.RuleIndex, r *rule.Rule, lib string) {
  log.Print(lib)
  sig := resolveDep(c, ix, fmt.Sprintf("virt:%s:%s_sig", lib, r.Name()))
  log.Print(sig)
}
