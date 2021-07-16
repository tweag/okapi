package okapi

import (
  "encoding/json"
  "flag"
  "os/exec"
  "path/filepath"
  "strings"

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

type Config struct { }

func (*okapiLang) Configure(c *config.Config, rel string, f *rule.File) {
  if f == nil {
    return
  }

  m, ok := c.Exts[okapiName]
  var extraConfig Config
  if ok {
    extraConfig = m.(Config)
  } else {
    extraConfig = Config{
    }
  }

  c.Exts[okapiName] = extraConfig
}

var defaultKind = rule.KindInfo {
  MatchAttrs: []string{},
  NonEmptyAttrs: map[string]bool{},
  MergeableAttrs: map[string]bool{
    "deps": true,
    "deps_opam": true,
  },
}

var kinds = map[string]rule.KindInfo {
  "ocaml_module": defaultKind,
  "ocaml_signature": defaultKind,
  "ocaml_ns_library": defaultKind,
}

func (*okapiLang) Kinds() map[string]rule.KindInfo { return kinds }

func (*okapiLang) Loads() []rule.LoadInfo {
  return []rule.LoadInfo{
    {
      Name:"@obazl_rules_ocaml//ocaml:rules.bzl",
      Symbols: []string{"ocaml_ns_library", "ocaml_module", "ocaml_signature"},
    },
  }
}

func (*okapiLang) Fix(c *config.Config, f *rule.File) {}

func (*okapiLang) Imports(c *config.Config, r *rule.Rule, f *rule.File) []resolve.ImportSpec {
  return []resolve.ImportSpec{{Lang: okapiName, Imp: r.Name()}}
}

func (*okapiLang) Embeds(r *rule.Rule, from label.Label) []label.Label { return nil }

func (*okapiLang) Resolve(c *config.Config, ix *resolve.RuleIndex, rc *repo.RemoteCache, r *rule.Rule, imports interface{}, from label.Label) {
}

var emptyResult = language.GenerateResult{
  Gen: []*rule.Rule{},
  Imports: []interface{}{},
}

type CodeptDep struct {
  File string
  Deps [][]string
}

type CodeptLocal struct {
  Module []string
  Ml string
  Mli string
}

type Codept struct {
  Dependencies []CodeptDep
  Local []CodeptLocal
}

type Source struct {
  Name string
  Intf bool
}

type Deps = map[Source][]string

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

func depName(file string) string {
  return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
}

func consSource(name string, sigs map[string][]string) Source {
  _, intf := sigs[name]
  return Source{name, intf}
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

func consDeps(codept Codept) Deps {
  local := make(map[string]string)
  sigs := make(map[string][]string)
  mods := make(map[string][]string)
  sources := make(Deps)
  for _, loc := range codept.Local {
    for _, mod := range loc.Module { local[mod] = depName(loc.Ml) }
  }
  for _, src := range codept.Dependencies {
    var deps []string
    for _, ds := range src.Deps {
      for _, d := range ds { deps = append(deps, local[d]) }
    }
    name := depName(src.File)
    if filepath.Ext(src.File) == ".mli" { sigs[name] = deps } else { mods[name] = deps }
  }
  for src, deps := range mods { sources[consSource(src, sigs)] = deps }
  return sources
}

func libraryRules(sources Deps) language.GenerateResult {
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

func generateLibrary(args language.GenerateArgs) language.GenerateResult {
  cmd := exec.Command("codept", "-native", "-deps", args.Dir)
  out, err := cmd.CombinedOutput()
  if err != nil { return emptyResult }
  var codept Codept
  json.Unmarshal(out, &codept)
  sources := consDeps(codept)
  return libraryRules(sources)
}

func (*okapiLang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
  for _, file := range args.RegularFiles {
    ext := filepath.Ext(file)
    if ext == ".ml" || ext == ".mli" {
      return generateLibrary(args)
    }
  }
  return emptyResult
}
