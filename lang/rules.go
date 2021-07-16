package okapi

import (
  "github.com/bazelbuild/bazel-gazelle/language"
  "github.com/bazelbuild/bazel-gazelle/rule"
)

func targetNames(deps []string) []string {
  var result []string
  for _, dep := range deps { result = append(result, ":" + dep) }
  return result
}

func sigTarget(src Source) string { return src.Name + "_sig" }

func genRule(kind string, name string, deps []string) *rule.Rule {
  r := rule.NewRule(kind, name)
  if len(deps) > 0 { r.SetAttr("deps", targetNames(deps)) }
  return r
}

func moduleRule(src Source, deps []string) *rule.Rule {
  r := genRule("ocaml_module", src.Name, deps)
  r.SetAttr("struct", ":" + src.Name + ".ml")
  if src.Intf { r.SetAttr("sig", ":" + sigTarget(src)) }
  return r
}

func signatureRule(src Source, deps []string) *rule.Rule {
  r := genRule("ocaml_signature", sigTarget(src), deps)
  r.SetAttr("src", ":" + src.Name + ".mli")
  return r
}

func libraryDep(src Source) string { return ":" + src.Name }

func libraryDeps(sources Deps) []string {
  var deps []string
  for src := range sources { deps = append(deps, libraryDep(src)) }
  return deps
}

func libraryRule(sources Deps) *rule.Rule {
  r := rule.NewRule("ocaml_ns_library", "#A")
  r.SetAttr("visibility", []string{"//visibility:public"})
  r.SetAttr("submodules", libraryDeps(sources))
  return r
}

func LibraryRules(sources Deps) language.GenerateResult {
  var rules []*rule.Rule
  var imports []interface{}
  for src, deps := range sources {
    rules = append(rules, moduleRule(src, deps))
    if src.Intf { rules = append(rules, signatureRule(src, deps)) }
  }
  rules = append(rules, libraryRule(sources))
  for range rules { imports = append(imports, 0) }
  return language.GenerateResult{ Gen: rules, Imports: imports }
}
