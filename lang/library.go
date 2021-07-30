package okapi

import (
	"sort"

	"github.com/bazelbuild/bazel-gazelle/rule"
)

type ModuleAlt struct {
  Cond string
  Choice string
}

type ModuleChoice struct {
  Out string
  Alts []ModuleAlt
}

type PpxKind interface {
  exe(name string) []*rule.Rule
}
type PpxTransitive struct {}
type PpxDirect struct { Deps []string }

func (ppx PpxDirect) exe(slug string) []*rule.Rule { return []*rule.Rule{ppxExecutable(slug, ppx.Deps)} }
func (PpxTransitive) exe(string) []*rule.Rule { return nil }

type Kind interface {
  libraryRuleName() string
  moduleRuleName() string
  moduleAttr() string
  addAttrs(slug string, r *rule.Rule) *rule.Rule
  extraRules(name string) []*rule.Rule
  wrapped() bool
}

type KindNsPpx struct { ppx PpxKind }
type KindNs struct {}
type KindPpx struct { ppx PpxKind }
type KindPlain struct {}

func (KindNsPpx) libraryRuleName() string { return "ppx_ns_library" }
func (KindNs) libraryRuleName() string { return "ocaml_ns_library" }
func (KindPpx) libraryRuleName() string { return "ppx_library" }
func (KindPlain) libraryRuleName() string { return "ocaml_library" }

func (KindNsPpx) moduleRuleName() string { return "ppx_module" }
func (KindNs) moduleRuleName() string { return "ocaml_module" }
func (KindPpx) moduleRuleName() string { return "ppx_module" }
func (KindPlain) moduleRuleName() string { return "ocaml_module" }

func (KindNsPpx) moduleAttr() string { return "submodules" }
func (KindNs) moduleAttr() string { return "submodules" }
func (KindPpx) moduleAttr() string { return "modules" }
func (KindPlain) moduleAttr() string { return "modules" }

func ppxName(libName string) string { return "ppx_" + libName }

func addPpxAttrs(slug string, r *rule.Rule) *rule.Rule {
  r.SetAttr("ppx", ":" + ppxName(slug))
  r.SetAttr("ppx_print", "@ppx//print:text")
  return r
}

func (KindNsPpx) addAttrs(slug string, r *rule.Rule) *rule.Rule { return addPpxAttrs(slug, r) }
func (KindNs) addAttrs(slug string, r *rule.Rule) *rule.Rule { return r }
func (KindPpx) addAttrs(slug string, r *rule.Rule) *rule.Rule { return addPpxAttrs(slug, r) }
func (KindPlain) addAttrs(slug string, r *rule.Rule) *rule.Rule { return r }

func ppxExecutable(name string, deps []string) *rule.Rule {
  r := rule.NewRule("ppx_executable", ppxName(name))
  r.SetAttr("deps_opam", deps)
  r.SetAttr("main", "@obazl_rules_ocaml//dsl:ppx_driver")
  return r
}

func (k KindNsPpx) extraRules(slug string) []*rule.Rule { return k.ppx.exe(slug) }
func (KindNs) extraRules(string) []*rule.Rule { return nil }
func (k KindPpx) extraRules(slug string) []*rule.Rule { return k.ppx.exe(slug) }
func (KindPlain) extraRules(string) []*rule.Rule { return nil }

func (KindNsPpx) wrapped() bool { return true }
func (KindNs) wrapped() bool { return true }
func (KindPpx) wrapped() bool { return false }
func (KindPlain) wrapped() bool { return false }

type Library struct {
  Slug string
  Name string
  Modules []string
  Opts []string
  DepsOpam []string
  Choices []ModuleChoice
  Auto bool
  Wrapped bool
  Kind Kind
}

func targetNames(deps []string) []string {
  var result []string
  for _, dep := range deps { result = append(result, ":" + dep) }
  sort.Strings(deps)
  return result
}

func commonAttrs(lib Library, r *rule.Rule, deps []string) *rule.Rule {
  r.SetAttr("deps_opam", lib.DepsOpam)
  r.SetAttr("opts", lib.Opts)
  if len(deps) > 0 { r.SetAttr("deps", targetNames(deps)) }
  return lib.Kind.addAttrs(lib.Slug, r)
}

func sigTarget(src Source) string { return src.Name + "_sig" }

func libSignatureRule(src Source) *rule.Rule {
  r := rule.NewRule("ocaml_signature", sigTarget(src))
  r.SetAttr("src", ":" + src.Name + ".mli")
  return r
}

func libModuleRule(lib Library, src Source) *rule.Rule {
  r := rule.NewRule(lib.Kind.moduleRuleName(), src.Name)
  r.SetAttr("struct", ":" + src.Name + ".ml")
  if src.Intf { r.SetAttr("sig", ":" + sigTarget(src)) }
  return r
}

func libSourceRules(sources Deps, lib Library) []*rule.Rule {
  var rules []*rule.Rule
  rules = append(rules, lib.Kind.extraRules(lib.Slug)...)
  for _, name := range lib.Modules {
    src, srcExists := sources[name]
    if !srcExists { src = Source{name, false, nil} }
    if src.Intf { rules = append(rules, commonAttrs(lib, libSignatureRule(src), src.Deps)) }
    rules = append(rules, commonAttrs(lib, libModuleRule(lib, src), src.Deps))
  }
  return rules
}

func setLibraryModules(lib Library, r *rule.Rule) {
  r.SetAttr(lib.Kind.moduleAttr(), targetNames(lib.Modules))
  r.SetAttr("visibility", []string{"//visibility:public"})
}

func libraryRule(lib Library) *rule.Rule {
  r := rule.NewRule(lib.Kind.libraryRuleName(), lib.Name)
  setLibraryModules(lib, r)
  if lib.Auto { r.AddComment("# okapi:auto") }
  return r
}

func library(sources Deps, lib Library) []*rule.Rule {
  return append(libSourceRules(sources, lib), libraryRule(lib))
}

// Update an existing build that has been manually amended by the user to contain more than one library.
// In that case, all submodule assignments are static, and only the module/signature rules are updated.
// TODO when `select` directives are used from dune, they don't create module rules for the choices.
// When gazelle is then run in update mode, they will be created.
// Either check for rules that select one of the choices or add exclude rules in comments.
func multilib(libs []Library, sources Deps, auto []string) []*rule.Rule {
  var rules []*rule.Rule
  for _, lib := range libs {
    if lib.Auto { lib.Modules = append(lib.Modules, auto...) }
    rules = append(rules, library(sources, lib)...)
  }
  return rules
}

func libChoices(libs []Library) map[string]bool {
  result := make(map[string]bool)
  for _, lib := range libs {
    for _, c := range lib.Choices {
      for _, a := range c.Alts {
        result[depName(a.Choice)] = true
      }
    }
  }
  return result
}

func autoModules(libs []Library, sources Deps) []string {
  knownModules := make(map[string]bool)
  choices := libChoices(libs)
  var auto []string
  for _, lib := range libs {
    for _, mod := range lib.Modules { knownModules[mod] = true }
  }
  for name := range sources {
    if _, exists := knownModules[name]; !exists {
      if _, isChoice := choices[name]; !isChoice {
        auto = append(auto, name)
      }
    }
  }
  return auto
}
