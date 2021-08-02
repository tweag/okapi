package okapi

import (
  "log"
  "path/filepath"
  "regexp"
  "sort"
  "strings"

  "github.com/bazelbuild/bazel-gazelle/language"
  "github.com/bazelbuild/bazel-gazelle/rule"
)

func generateLibraryName(dir string) string {
  return "#" + strings.ReplaceAll(strings.Title(filepath.Base(dir)), "-", "_")
}

func GenerateRulesAuto(name string, sources Deps) []RuleResult {
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

func GenerateRulesDune(name string, sources Deps, duneCode string) []RuleResult {
  conf := parseDuneFile(duneCode)
  duneLibs := DecodeDuneConfig(name, conf)
  var libs []Library
  for _, dune := range duneLibs {
    libs = append(libs, duneToLibrary(dune))
  }
  auto := autoModules(libs, sources)
  return multilib(libs, sources, auto)
}

func GenerateRules(name string, sources Deps, dune string) []RuleResult {
  if dune == "" { return GenerateRulesAuto(name, sources) } else { return GenerateRulesDune(name, sources, dune) }
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

// TODO need to fill in the deps somewhere after parsing, by looking for the corresponding ppx_executable
// maybe that can even be skipped by passing around a flag `UpdateMode` and not emitting a ppx_executable when true,
// since it would just generate the identical rule again.
var libKinds = map[string]Kind {
  "ocaml_ns_library": KindNs{},
  "ppx_ns_library": KindNsPpx{},
  "ocaml_library": KindPlain{},
  "ppx_library": KindPpx{},
}

func isLibrary(r *rule.Rule) bool {
  _, isLib := libKinds[r.Kind()]
  return isLib
}

var sourceKinds = map[string]bool {
  "ocaml_signature": true,
  "ocaml_module": true,
  "ppx_module": true,
}

func isSource(r *rule.Rule) bool {
  _, isSrc := sourceKinds[r.Kind()]
  return isSrc
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

func AmendRules(args language.GenerateArgs, rules []*rule.Rule, sources Deps) []RuleResult {
  libs, auto := existingLibraries(rules, sources)
  return multilib(libs, sources, auto)
}
