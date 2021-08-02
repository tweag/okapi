package okapi

import (
  "flag"
  "log"
  "path/filepath"

  "github.com/bazelbuild/bazel-gazelle/config"
  "github.com/bazelbuild/bazel-gazelle/label"
  "github.com/bazelbuild/bazel-gazelle/language"
  "github.com/bazelbuild/bazel-gazelle/repo"
  "github.com/bazelbuild/bazel-gazelle/resolve"
  "github.com/bazelbuild/bazel-gazelle/rule"
)

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

func importSpec(name string) resolve.ImportSpec {
  return resolve.ImportSpec{Lang: okapiName, Imp: name}
}

type ResolvedLocal struct { label label.Label }
type ResolvedOpam struct {}

func resolveDep(c *config.Config, ix *resolve.RuleIndex, dep string) interface{} {
  results := ix.FindRulesByImportWithConfig(c, importSpec(generateLibraryName(dep)), okapiName)
  if len(results) == 0 {
    return ResolvedOpam{}
  } else if len(results) == 1 {
    return ResolvedLocal{results[0].Label}
  } else {
    log.Fatal("Multiple libraries matched the depspec `" + dep + "`")
    return nil
  }
}

func (*okapiLang) Imports(c *config.Config, r *rule.Rule, f *rule.File) []resolve.ImportSpec {
  if isLibrary(r) {
    return []resolve.ImportSpec{importSpec(r.Name())}
  } else {
    return nil
  }
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
  findDep := func (dep string) interface{} {
    return resolveDep(c, ix, dep)
  }
  if isSource(r) {
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
}

func containsLibrary(rules []*rule.Rule) bool {
  for _, r := range rules {
    if isLibrary(r) { return true }
  }
  return false
}

var emptyResult = language.GenerateResult{
  Gen: []*rule.Rule{},
  Empty: []*rule.Rule{},
  Imports: []interface{}{},
}

func containsOcaml(args language.GenerateArgs) bool {
  for _, file := range args.RegularFiles {
    ext := filepath.Ext(file)
    if ext == ".ml" || ext == ".mli" {
      return true
    }
  }
  return false
}

func generateIfOcaml(args language.GenerateArgs) []RuleResult {
  if containsOcaml(args) {
    return GenerateRules(
      args.Dir,
      Dependencies(args.Dir, args.RegularFiles),
      findDune(args.Dir, args.RegularFiles),
    )
  } else {
    return nil
  }
}

func (*okapiLang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
  var results []RuleResult
  if args.File != nil && args.File.Rules != nil && containsLibrary(args.File.Rules) {
    results = AmendRules(args, args.File.Rules, Dependencies(args.Dir, args.RegularFiles))
  } else {
    results = generateIfOcaml(args)
  }
  var rules []*rule.Rule
  var imports []interface{}
  for _, result := range results {
    rules = append(rules, result.rule)
    imports = append(imports, result.deps)
  }
  return language.GenerateResult{
    Gen: rules,
    Empty: nil,
    Imports: imports,
  }
}
