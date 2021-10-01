package okapi

import (
  "flag"
  "fmt"
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

type Config struct {
  library *bool
}

// Entry point to Gazelle
func NewLanguage() language.Language { return &okapiLang{} }

func (*okapiLang) Name() string { return okapiName }

func (*okapiLang) RegisterFlags(fs *flag.FlagSet, cmd string, c *config.Config) {
  library := fs.Bool("library", false, "build libraries instead of archives")
  c.Exts[okapiName] = Config {
    library: library,
  }
}

func (*okapiLang) CheckFlags(fs *flag.FlagSet, c *config.Config) error { return nil }

func (*okapiLang) KnownDirectives() []string { return []string{} }

func (*okapiLang) Configure(c *config.Config, rel string, f *rule.File) {}

// Related to merge
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
  "ppx_ns_archive": defaultKind,
  "ppx_archive": defaultKind,
  "ocaml_ns_archive": defaultKind,
  "ocaml_archive": defaultKind,
  "filegroup": defaultKind,
  "ocaml_executable": defaultKind,
  "ppx_executable": defaultKind,
  "ocaml_test": defaultKind,
  "ppx_test": defaultKind,
  "ocaml_lex": defaultKind,
}

func (*okapiLang) Kinds() map[string]rule.KindInfo { return kinds }

// Load OBazl stuff (functions)
func (*okapiLang) Loads() []rule.LoadInfo {
  return []rule.LoadInfo{
    {
      Name: "@obazl_rules_ocaml//ocaml:rules.bzl",
      Symbols: []string{
        "ocaml_ns_library",
        "ocaml_library",
        "ppx_ns_library",
        "ppx_library",
        "ocaml_ns_archive",
        "ocaml_archive",
        "ppx_ns_archive",
        "ppx_archive",
        "ocaml_module",
        "ppx_module",
        "ocaml_signature",
        "ocaml_executable",
        "ppx_executable",
        "ocaml_test",
        "ppx_test",
        "ocaml_lex",
      },
      After: nil,
    },
  }
}

func (*okapiLang) Fix(c *config.Config, f *rule.File) {}

// Build the dictionary of libraries (not Opam dependencies) that will be used for dep resolution afterwards
func (*okapiLang) Imports(c *config.Config, r *rule.Rule, f *rule.File) []resolve.ImportSpec {
  var imports []resolve.ImportSpec
  if isLibrary(r) {
    imports = append(imports, importSpec(r.Name()))
    if name, exists := ruleConfig(r, "public_name"); exists {
      imports = append(imports, importSpec(name))
    }
    if name, exists := ruleConfig(r, "implements"); exists {
      imports = append(imports, importSpec("impl:" + name))
    }
    if name, exists := ruleConfig(r, "implementation"); exists {
      imports = append(imports, importSpec("implementation:" + name))
    }
  } else if isSignature(r) {
    if lib, exists := ruleConfig(r, "virt"); exists {
      imports = append(imports, importSpec(fmt.Sprintf("virt:%s", lib)))
      imports = append(imports, importSpec(fmt.Sprintf("virt:%s:%s", lib, r.Name())))
    }
  }
  return imports
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
  if isSource(r) { libraryDeps(c, ix, imports, r) }
  if isExecutable(r) { executableDeps(c, ix, imports, r) }
}

func containsLibrary(rules []*rule.Rule) bool {
  for _, r := range rules { if isLibrary(r) { return true } }
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
    if ext == ".ml" || ext == ".mli" { return true }
  }
  return false
}

func generateIfOcaml(args language.GenerateArgs, library bool) []RuleResult {
  if containsOcaml(args) {
    return GenerateRules(
      args.Dir,
      Dependencies(args.Dir, args.RegularFiles),
      findDune(args.Dir, args.RegularFiles),
      library,
    )
  } else {
    return nil
  }
}

// Main entry point for Okapi.
func (*okapiLang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
  config, valid := args.Config.Exts[okapiName].(Config)
  if !valid { log.Fatalf("invalid config: %#v", args.Config.Exts[okapiName]) }
  var results []RuleResult
  if args.File != nil && args.File.Rules != nil && containsLibrary(args.File.Rules) {
    results = AmendRules(args, args.File.Rules, Dependencies(args.Dir, args.RegularFiles), *config.library)
  } else {
    results = generateIfOcaml(args, *config.library)
  }
	// Poorman's unzip
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
