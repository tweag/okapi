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
  }
  return nil
}

func appendLabels(r *rule.Rule, attr string, deps []label.Label) {
  var names []string
  for _, d := range deps {
    names = append(names, d.String())
  }
  if len(deps) > 0 { r.SetAttr(attr, append(r.AttrStrings(attr), names...)) }
}

// If the `sig` attr for the module implementing a virtual module isn't set, a `.mli` will be generated and `ocamlfind`
// will print a warning due to multiple `.cmi` files in the include path.
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
    extendAttr(r, "deps", locals)
    extendAttr(r, "deps_opam", opams)
  } else {
    log.Fatalf("Invalid type for imports of source file %s: %#v", r.Name(), imports)
  }
  if virt, exists := ruleConfig(r, "implements"); exists && r.AttrString("sig") == "" {
    for _, sig := range findImport(c, ix, fmt.Sprintf("virt:%s:%s", virt, r.Name())) {
      r.SetAttr("sig", sig.Label.String())
    }
  }
}

func executableDeps(
  c *config.Config,
  ix *resolve.RuleIndex,
  imports interface{},
  r *rule.Rule,
) {
  if deps, isStrings := imports.([]string); isStrings {
    var impls []string
    for _, dep := range deps {
      for _, lib := range findImport(c, ix, fmt.Sprintf("implementation:%s", dep)) {
        impls = append(impls, lib.Label.String())
      }
    }
    extendAttr(r, "deps", impls)
  } else {
    log.Fatalf("Invalid type for imports of executable %s: %#v", r.Name(), imports)
  }
}
