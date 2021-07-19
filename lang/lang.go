package okapi

import (
  "flag"
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

type Config struct { }

func (*okapiLang) Configure(c *config.Config, rel string, f *rule.File) {
  if f == nil { return }
  m, ok := c.Exts[okapiName]
  var extraConfig Config
  if ok { extraConfig = m.(Config)
  } else { extraConfig = Config{ } }
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

func (*okapiLang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
  for _, file := range args.RegularFiles {
    ext := filepath.Ext(file)
    if ext == ".ml" || ext == ".mli" {
      return LibraryRules(Dependencies(args.Dir))
    }
  }
  return emptyResult
}
