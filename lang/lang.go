package okapi

import (
  "flag"
  "log"
  "path/filepath"
  "regexp"
  "sort"
  "strings"

  "github.com/bazelbuild/bazel-gazelle/config"
  "github.com/bazelbuild/bazel-gazelle/label"
  "github.com/bazelbuild/bazel-gazelle/language"
  "github.com/bazelbuild/bazel-gazelle/repo"
  "github.com/bazelbuild/bazel-gazelle/resolve"
  "github.com/bazelbuild/bazel-gazelle/rule"
)

// -----------------------------------------------------------------------------
// Library
// -----------------------------------------------------------------------------

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

func commonAttrs(lib Library, r *rule.Rule, deps []string) *rule.Rule {
  r.SetAttr("deps_opam", lib.DepsOpam)
  r.SetAttr("opts", lib.Opts)
  if len(deps) > 0 { r.SetAttr("deps", targetNames(deps)) }
  return lib.Kind.addAttrs(lib.Slug, r)
}

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

// -----------------------------------------------------------------------------
// Rules
// -----------------------------------------------------------------------------

func generateLibraryName(dir string) string {
  return "#" + strings.ReplaceAll(strings.Title(filepath.Base(dir)), "-", "_")
}

func targetNames(deps []string) []string {
  var result []string
  for _, dep := range deps { result = append(result, ":" + dep) }
  sort.Strings(deps)
  return result
}

func sigTarget(src Source) string { return src.Name + "_sig" }

func resultForRules(rules []*rule.Rule) language.GenerateResult {
  var imports []interface{}
  for range rules { imports = append(imports, 0) }
  return language.GenerateResult{
    Gen: rules,
    Empty: []*rule.Rule{},
    Imports: imports,
  }
}

func GenerateRulesAuto(name string, sources Deps) []*rule.Rule {
  var keys []string
  for key := range sources { keys = append(keys, key) }
  sort.Strings(keys)
  lib := Library{
    Slug: name,
    Name: generateLibraryName(name),
    Modules: keys,
    Opts: nil,
    DepsOpam: nil,
    Choices: nil,
    Auto: true,
    Wrapped: false,
    Kind: KindNs{},
  }
  return library(sources, lib)
}

func GenerateRulesDune(name string, sources Deps, duneCode string) []*rule.Rule {
  conf := parseDuneFile(duneCode)
  duneLibs := DecodeDuneConfig(name, conf)
  var libs []Library
  for _, dune := range duneLibs {
    libs = append(libs, duneToLibrary(dune))
  }
  auto := autoModules(libs, sources)
  return multilib(libs, sources, auto)
}

func GenerateRules(name string, sources Deps, dune string) []*rule.Rule {
  if dune == "" { return GenerateRulesAuto(name, sources) } else { return GenerateRulesDune(name, sources, dune) }
}

var EmptyResult = language.GenerateResult{
  Gen: nil,
  Empty: nil,
  Imports: nil,
}

func tags(r *rule.Rule) []string {
  var tags []string
  rex := regexp.MustCompile(`^# okapi:(\S+)`)
  for _, c := range r.Comments() {
    match := rex.FindStringSubmatch(c)
    if len(match) == 1 { tags = append(tags, match[0]) }
  }
  return tags
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

// TODO need to fill in the deps somewhere after parsing, by looking for the corresponding ppx_executable
// maybe that can even be skipped by passing around a flag `UpdateMode` and not emitting a ppx_executable when true,
// since it would just generate the identical rule again.
var libKinds = map[string]Kind {
  "ocaml_ns_library": KindNs{},
  "ppx_ns_library": KindNsPpx{},
  "ocaml_library": KindPlain{},
  "ppx_library": KindPpx{},
}

func slug(name string) string {
  rex := regexp.MustCompile("#([[:upper:]])(.*)")
  match := rex.FindStringSubmatch(name)
  if len(match) != 2 { log.Fatal("Library name " + name + " couldn't be parsed.'") }
  return strings.ToLower(match[0]) + match[1]
}

func removeColon(name string) string {
  if name[:1] == ":" { return name[1:] } else {return name}
}

// TODO this uses only the `deps` names that correspond to existing source files.
// It should also consider generated sources, like from selects and manual rules.
// Probably needs to scan existing module rules as well.
// TODO general question about attrs like opts and deps_opam: is it more sensible to leave these nil when updating,
// since they get merged anyway?
// TODO `deps` is wrong, this needs to use `submodules` or `modules` (`kind.moduleAttr`)
func existingLibrary(r *rule.Rule, sources Deps) (Library, bool) {
  if kind, isLib := libKinds[r.Name()]; isLib {
    var modules []string
    for _, name := range r.AttrStrings("deps") {
      clean := removeColon(name)
      if _, exists := sources[clean]; exists { modules = append(modules, clean) }
    }
    lib := Library{
      Slug: slug(r.Name()),
      Name: r.Name(),
      Modules: modules,
      Opts: nil,
      DepsOpam: nil,
      Choices: nil,
      Auto: contains("auto", tags(r)),
      Wrapped: kind.wrapped(),
      Kind: kind,
    }
    return lib, true
  }
  return Library{}, false
}

func existingLibraries(rules []*rule.Rule, sources Deps) ([]Library, []string) {
  var libs []Library
  for _, r := range rules {
    if lib, isLib := existingLibrary(r, sources); isLib { libs = append(libs, lib) }
  }
  return libs, autoModules(libs, sources)
}

func AmendRules(args language.GenerateArgs, rules []*rule.Rule, sources Deps) []*rule.Rule {
  libs, auto := existingLibraries(rules, sources)
  return multilib(libs, sources, auto)
}

// -----------------------------------------------------------------------------
// Language
// -----------------------------------------------------------------------------

const okapiName = "okapi"

type okapiLang struct {}

func NewLanguage() language.Language { return &okapiLang{} }

func (*okapiLang) Name() string { return okapiName }

func (*okapiLang) RegisterFlags(fs *flag.FlagSet, cmd string, c *config.Config) {}

func (*okapiLang) CheckFlags(fs *flag.FlagSet, c *config.Config) error { return nil }

func (*okapiLang) KnownDirectives() []string { return []string{} }

type Config struct {}

func (*okapiLang) Configure(c *config.Config, rel string, f *rule.File) {
  if f == nil { return }
  m, ok := c.Exts[okapiName]
  var extraConfig Config
  if ok { extraConfig = m.(Config)
  } else { extraConfig = Config{ } }
  c.Exts[okapiName] = extraConfig
}

var defaultKind = rule.KindInfo{
  MatchAny: false,
  MatchAttrs: []string{},
  NonEmptyAttrs: map[string]bool{},
  SubstituteAttrs: map[string]bool{},
  MergeableAttrs: map[string]bool{"submodules": true, "modules": true},
  ResolveAttrs: map[string]bool{},
}

var kinds = map[string]rule.KindInfo {
  "ppx_module": defaultKind,
  "ocaml_module": defaultKind,
  "ocaml_signature": defaultKind,
  "ppx_ns_library": defaultKind,
  "ppx_library": defaultKind,
  "ocaml_ns_library": defaultKind,
  "ocaml_library": defaultKind,
}

func (*okapiLang) Kinds() map[string]rule.KindInfo { return kinds }

func (*okapiLang) Loads() []rule.LoadInfo {
  return []rule.LoadInfo{
    {
      Name: "@obazl_rules_ocaml//ocaml:rules.bzl",
      Symbols: []string{
        "ocaml_ns_library",
        "ppx_ns_library",
        "ocaml_module",
        "ppx_module",
        "ocaml_signature",
        "ppx_executable",
      },
      After: nil,
    },
  }
}

func (*okapiLang) Fix(c *config.Config, f *rule.File) {}

func (*okapiLang) Imports(c *config.Config, r *rule.Rule, f *rule.File) []resolve.ImportSpec {
  return []resolve.ImportSpec{{Lang: okapiName, Imp: r.Name()}}
}

func (*okapiLang) Embeds(r *rule.Rule, from label.Label) []label.Label { return nil }

func (*okapiLang) Resolve(
  c *config.Config,
  ix *resolve.RuleIndex,
  rc *repo.RemoteCache,
  r *rule.Rule,
  imports interface{},
  from label.Label,
) {
}

func containsLibrary(rules []*rule.Rule) bool {
  for _, r := range rules {
    if _, isLib := libKinds[r.Kind()]; isLib { return true }
  }
  return false
}

func generateIfOcaml(args language.GenerateArgs) []*rule.Rule {
  for _, file := range args.RegularFiles {
    ext := filepath.Ext(file)
    if ext == ".ml" || ext == ".mli" {
      return GenerateRules(
        args.Dir,
        Dependencies(args.Dir, args.RegularFiles),
        findDune(args.Dir, args.RegularFiles),
      )
    }
  }
  return nil
}

func (*okapiLang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
  var rules []*rule.Rule
  var imports []interface{}
  if args.File != nil && args.File.Rules != nil && containsLibrary(args.File.Rules) {
    rules = AmendRules(args, args.File.Rules, Dependencies(args.Dir, args.RegularFiles))
  } else {
    rules = generateIfOcaml(args)
  }
  for range rules { imports = append(imports, 0) }
  return resultForRules(rules)
}
