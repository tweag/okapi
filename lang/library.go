package okapi

import (
  "fmt"
  "sort"
  "strings"

  "github.com/bazelbuild/bazel-gazelle/rule"
)

type KeyValue struct {
  key string
  value string
}

type ModuleAlt struct {
  Cond string
  Choice string
}

type ModuleChoice struct {
  Out string
  Alts []ModuleAlt
}

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

type PpxTransitive struct {}
type PpxDirect struct { deps []string }
type NoPpx struct {}

func (PpxTransitive) exe(string) []RuleResult { return nil }
func (ppx PpxDirect) exe(slug string) []RuleResult {
  return []RuleResult{{ppxExecutable(slug, ppx.deps), nil}}
}
func (NoPpx) exe(string) []RuleResult { return nil }

func (PpxTransitive) depsOpam() []string { return nil }
func (ppx PpxDirect) depsOpam() []string { return ppx.deps }
func (NoPpx) depsOpam() []string { return nil }

func (PpxTransitive) isPpx() bool { return true }
func (PpxDirect) isPpx() bool { return true }
func (NoPpx) isPpx() bool { return false }

func ppxName(libName string) string { return "ppx_" + libName }

func addAttrs(slug string, r *rule.Rule, kind PpxKind) {
  if ppx, isDirect := kind.(PpxDirect); isDirect {
    r.SetAttr("ppx", ":" + ppxName(slug))
    r.SetAttr("ppx_print", "@ppx//print:text")
    if contains("ppx_inline_test", ppx.deps) { r.SetAttr("ppx_tags", []string{"inline-test"}) }
  }
}

func extraRules(kind PpxKind, slug string) []RuleResult {
  if ppx, isDirect := kind.(PpxDirect); isDirect { return ppx.exe(slug) }
  return nil
}

func nsLibraryName(name string) string {
  return "#" + strings.Title(strings.ReplaceAll(name, "-", "_"))
}

type LibraryKind interface {
  ruleKind() string
  ppx() bool
  wrapped() bool
}

type LibNsPpx struct {}
type LibNs struct {}
type LibPpx struct {}
type LibPlain struct {}

func (LibNsPpx) ruleKind() string { return "ppx_ns_library" }
func (LibNs) ruleKind() string { return "ocaml_ns_library" }
func (LibPpx) ruleKind() string { return "ppx_library" }
func (LibPlain) ruleKind() string { return "ocaml_library" }

func (LibNsPpx) ppx() bool { return true }
func (LibNs) ppx() bool { return false }
func (LibPpx) ppx() bool { return true }
func (LibPlain) ppx() bool { return false }

func (LibNsPpx) wrapped() bool { return true }
func (LibNs) wrapped() bool { return true }
func (LibPpx) wrapped() bool { return false }
func (LibPlain) wrapped() bool { return false }

type ExeKind interface {
  ruleKind() string
  ppx() bool
}

type ExePpx struct {}
type ExePlain struct {}

func (ExePpx) ruleKind() string { return "ppx_executable" }
func (ExePlain) ruleKind() string { return "ocaml_executable" }

func (ExePpx) ppx() bool { return true }
func (ExePlain) ppx() bool { return false }

type ComponentKind interface {
  componentRule(component Component) *rule.Rule
  extraDeps() []string
}

type Library struct {
  virtualModules []string
  implements string
  kind LibraryKind
}

type Executable struct {
  kind ExeKind
}

type Generated interface {
  target() string
  moduleDep() bool
  rules(Component) []RuleResult
}
type Lex struct {
  name string
}

func (lex Lex) target() string { return lex.name }

func (lex Lex) moduleDep() bool { return true }

func (lex Lex) rules(component Component) []RuleResult {
  structName := lex.name + "_ml"
  lexRule := rule.NewRule("ocaml_lex", structName)
  lexRule.SetAttr("src", ":" + lex.name + ".mll")
  modRule := moduleRule(component, Source{lex.name, false, false, nil}, ":" + structName, nil)
  return []RuleResult{{lexRule, nil}, modRule}
}

type Component struct {
  name string
  publicName string
  modules []string
  opts []string
  depsOpam []string
  choices []ModuleChoice
  auto bool
  ppx PpxKind
  generated []Generated
  kind ComponentKind
}

func moduleAttr(wrapped bool) string {
  if wrapped { return "submodules" } else { return "modules" }
}

func nsName(name string) string {
  return "#" + strings.Title(strings.ReplaceAll(name, "-", "_"))
}

func targetNames(deps []string) []string {
  var result []string
  for _, dep := range deps { result = append(result, ":" + dep) }
  sort.Strings(result)
  return result
}

func (lib Library) componentRule(component Component) *rule.Rule {
  libName := "lib-" + component.name
  if lib.kind.wrapped() { libName = nsName(component.name) }
  r := rule.NewRule(lib.kind.ruleKind(), libName)
  mods := append(component.modules, lib.virtualModules...)
  for _, gen := range component.generated {
    mods = append(mods, gen.target())
  }
  r.SetAttr(moduleAttr(lib.kind.wrapped()), targetNames(mods))
  if lib.implements != "" {
    r.AddComment("# okapi:implements " + lib.implements)
    r.AddComment("# okapi:implementation " + component.publicName)
  }
  return r
}

func (exe Executable) componentRule(component Component) *rule.Rule {
  r := rule.NewRule(exe.kind.ruleKind(), component.publicName)
  r.SetAttr("main", component.name)
  return r
}

func (lib Library) extraDeps() []string {
  if lib.implements == "" { return nil } else { return []string{lib.implements} }
}

func (Executable) extraDeps() []string { return nil }

type RuleResult struct {
  rule *rule.Rule
  deps []string
}

func extendAttr(r *rule.Rule, attr string, vs []string) {
  if len(vs) > 0 { r.SetAttr(attr, append(r.AttrStrings(attr), vs...)) }
}

func appendAttr(r *rule.Rule, attr string, v string) {
  r.SetAttr(attr, append(r.AttrStrings(attr), v))
}

func commonAttrs(component Component, r *rule.Rule, deps []string) RuleResult {
  libDeps := append(append(component.depsOpam, component.ppx.depsOpam()...), component.kind.extraDeps()...)
  extendAttr(r, "opts", component.opts)
  if len(deps) > 0 { r.SetAttr("deps", targetNames(deps)) }
  addAttrs(component.name, r, component.ppx)
  return RuleResult{r, libDeps}
}

func sigTarget(src Source) string { return src.Name + "_sig" }

func signatureRule(component Component, src Source, deps []string) RuleResult {
  r := rule.NewRule("ocaml_signature", sigTarget(src))
  r.SetAttr("src", ":" + src.Name + ".mli")
  return commonAttrs(component, r, deps)
}

func virtualSignatureRule(libName string, src Source) *rule.Rule {
  r := rule.NewRule("ocaml_signature", src.Name)
  r.SetAttr("src", ":" + src.Name + ".mli")
  r.AddComment(fmt.Sprintf("# okapi:virt %s", libName))
  return r
}

func moduleRuleName(component Component) string {
  if component.ppx.isPpx() { return "ppx_module" } else { return "ocaml_module" }
}

func moduleRule(component Component, src Source, struct_ string, deps []string) RuleResult {
  r := rule.NewRule(moduleRuleName(component), src.Name)
  r.SetAttr("struct", struct_)
  if src.Intf {
    r.SetAttr("sig", ":" + sigTarget(src))
  } else if lib, isLib := component.kind.(Library); isLib && lib.implements != "" {
    r.AddComment(fmt.Sprintf("# okapi:implements %s", lib.implements))
  }
  return commonAttrs(component, r, deps)
}

func defaultModuleRule(component Component, src Source, deps []string) RuleResult {
  return moduleRule(component, src, ":" + src.Name + ".ml", deps)
}

func generatedDeps(generated []Generated) []string {
  var result []string
  for _, gen := range generated {
    if gen.moduleDep() { result = append(result, gen.target()) }
  }
  return result
}

func remove(name string, deps []string) []string {
  var result []string
  for _, dep := range deps {
    if dep != name { result = append(result, dep) }
  }
  return result
}

func librarySourceRules(component Component, lib Library, sources Deps) []RuleResult {
  var rules []RuleResult
  for _, name := range lib.virtualModules {
    src, srcExists := sources[name]
    if !srcExists { src = Source{name, false, false, nil} }
    cleanDeps := remove(name, src.Deps)
    rules = append(rules, commonAttrs(component, virtualSignatureRule(component.publicName, src), cleanDeps))
  }
  return rules
}

func sourceRules(sources Deps, component Component) []RuleResult {
  var rules []RuleResult
  rules = append(rules, extraRules(component.ppx, component.name)...)
  for _, name := range component.modules {
    src, srcExists := sources[name]
    if !srcExists { src = Source{name, false, false, nil} }
    cleanDeps := append(remove(name, src.Deps), generatedDeps(component.generated)...)
    if src.Intf { rules = append(rules, signatureRule(component, src, cleanDeps)) }
    rules = append(rules, defaultModuleRule(component, src, cleanDeps))
  }
  if lib, isLib := component.kind.(Library); isLib {
    rules = append(rules, librarySourceRules(component, lib, sources)...)
  }
  return rules
}

func setLibraryModules(component Component, r *rule.Rule) {
}

func componentRule(component Component) RuleResult {
  r := component.kind.componentRule(component)
  if component.auto { r.AddComment("# okapi:auto") }
  r.AddComment("# okapi:public_name " + component.publicName)
  r.SetAttr("visibility", []string{"//visibility:public"})
  return RuleResult{r, component.depsOpam}
}

func generators(component Component) []RuleResult {
  var result []RuleResult
  for _, gen := range component.generated {
    result = append(result, gen.rules(component)...)
  }
  return result
}

func component(sources Deps, component Component) []RuleResult {
  return append(generators(component), append(sourceRules(sources, component), componentRule(component))...)
}

// Update an existing build that has been manually amended by the user to contain more than one library.
// In that case, all submodule assignments are static, and only the module/signature rules are updated.
// TODO when `select` directives are used from dune, they don't create module rules for the choices.
// When gazelle is then run in update mode, they will be created.
// Either check for rules that select one of the choices or add exclude rules in comments.
func multilib(libs []Component, sources Deps, auto []string) []RuleResult {
  var rules []RuleResult
  for _, lib := range libs {
    if lib.auto { lib.modules = append(lib.modules, auto...) }
    rules = append(rules, component(sources, lib)...)
  }
  return rules
}

func libChoices(libs []Component) map[string]bool {
  result := make(map[string]bool)
  for _, lib := range libs {
    for _, c := range lib.choices {
      for _, a := range c.Alts {
        result[depName(a.Choice)] = true
      }
    }
  }
  return result
}

func autoModules(components []Component, sources Deps) []string {
  knownModules := make(map[string]bool)
  choices := libChoices(components)
  var auto []string
  for _, component := range components {
    for _, mod := range component.modules { knownModules[mod] = true }
    if lib, isLib := component.kind.(Library); isLib {
      for _, mod := range lib.virtualModules { knownModules[mod] = true }
    }
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
