package okapi

import (
  "log"
  "path/filepath"
  "regexp"
  "strings"

  "github.com/bazelbuild/bazel-gazelle/language"
  "github.com/bazelbuild/bazel-gazelle/rule"
)

func GenerateRulesAuto(name string, sources Deps, library bool) []RuleResult {
  var keys []string
  for key := range sources { keys = append(keys, key) }
  var modules []Source
  for _, key := range keys { modules = append(modules, sources[key]) }
  sortSources(modules)
  lib := Component{
    core: ComponentCore{
      name: name,
      publicName: name,
      flags: nil,
      auto: true,
    },
    modules: modules,
    depsOpam: nil,
    ppx: NoPpx{},
    kind: Library{
      virtualModules: nil,
      implements: "",
      kind: LibNs{},
    },
  }
  return component(sources, lib, library)
}

func GenerateRulesDune(name string, sources Deps, duneCode string, library bool) []RuleResult {
  conf := parseDuneFile(duneCode)
  duneConf := DecodeDuneConfig(name, conf)
  spec := duneToSpec(duneConf)
  return multilib(spec, sources, library)
}

func GenerateRules(dir string, sources Deps, dune string, library bool) []RuleResult {
  name := filepath.Base(dir)
  if dune == "" {
    return GenerateRulesAuto(name, sources, library)
  } else {
    return GenerateRulesDune(name, sources, dune, library)
  }
}

func tags(r *rule.Rule) []string {
  var tags []string
  rex := regexp.MustCompile(`^# okapi:(\S+)`)
  for _, c := range r.Comments() {
    match := rex.FindStringSubmatch(c)
    if len(match) == 2 { tags = append(tags, match[1]) }
  }
  return tags
}

func hasTag(name string, r *rule.Rule) bool {
  return contains(name, tags(r))
}

func ruleConfigs(r *rule.Rule) []KeyValue {
  var kvs []KeyValue
  rex := regexp.MustCompile(`^# okapi:(\S+) (\S.*)`)
  for _, c := range r.Comments() {
    match := rex.FindStringSubmatch(c)
    if len(match) == 3 { kvs = append(kvs, KeyValue{match[1], match[2]}) }
  }
  return kvs
}

func ruleConfig(r *rule.Rule, key string) (string, bool) {
  for _, kv := range ruleConfigs(r) {
    if kv.key == key { return kv.value, true }
  }
  return "", false
}

func ruleConfig2(r *rule.Rule, key string) (string, string, bool) {
  if annotation, exists := ruleConfig(r, key); exists {
    parts := strings.Split(annotation, " ")
    if len(parts) != 2 {
      log.Fatalf("Invalid `%s` annotation for %s: %s", key, r.Name(), annotation)
    }
    return parts[0], parts[1], true
  }
  return "", "", false
}

func ruleConfigOr(r *rule.Rule, key string, def string) string {
  if value, exists := ruleConfig(r, key); exists { return value } else { return def }
}

// TODO need to fill in the deps somewhere after parsing, by looking for the corresponding ppx_executable
// maybe that can even be skipped by passing around a flag `UpdateMode` and not emitting a ppx_executable when true,
// since it would just generate the identical rule again.
var libKinds = map[string]LibraryKind {
  "ocaml_ns_library": LibNs{},
  "ppx_ns_library": LibNsPpx{},
  "ocaml_library": LibPlain{},
  "ppx_library": LibPpx{},
  "ocaml_ns_archive": LibNs{},
  "ppx_ns_archive": LibNsPpx{},
  "ocaml_archive": LibPlain{},
  "ppx_archive": LibPpx{},
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
  _, isSource := sourceKinds[r.Kind()]
  return isSource
}

var moduleKinds = map[string]bool {
  "ocaml_signature": true,
  "ocaml_module": true,
  "ppx_module": true,
}

func isModule(r *rule.Rule) bool {
  _, isModule := moduleKinds[r.Kind()]
  return isModule
}

func isSignature(r *rule.Rule) bool {
  return r.Kind() == "ocaml_signature"
}

var executableKinds = map[string]bool {
  "ocaml_executable": true,
  "ppx_executable": true,
}

func isExecutable(r *rule.Rule) bool {
  _, isExecutable := executableKinds[r.Kind()]
  return isExecutable
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
func existingLibrary(r *rule.Rule, sources Deps) (ComponentSpec, bool) {
  if kind, isLib := libKinds[r.Name()]; isLib {
    var moduleSpec ModuleSpec = AutoModules{}
    if len(r.AttrStrings("deps")) > 0 {
      var modules []string
      for _, name := range r.AttrStrings("deps") {
        clean := removeColon(name)
        modules = append(modules, clean)
        // if src, exists := sources[clean]; exists { modules = append(modules, src) }
      }
      moduleSpec = ConcreteModules{modules}
    }
    nameSlug := slug(r.Name())
    publicName := ruleConfigOr(r, "public_name", nameSlug)
    implements := ruleConfigOr(r, "implements", "")
    var ppx PpxKind = NoPpx{}
    if kind.ppx() { ppx = PpxTransitive{} }
    lib := ComponentSpec{
      core: ComponentCore{
        name: nameSlug,
        publicName: publicName,
        flags: nil,
        auto: hasTag("auto", r),
      },
      modules: moduleSpec,
      depsOpam: nil,
      ppx: ppx,
      kind: LibSpec{
        wrapped: kind.wrapped(),
        virtualModules: nil,
        implements: implements,
      },
    }
    return lib, true
  }
  return ComponentSpec{}, false
}

func existingLibraries(rules []*rule.Rule, sources Deps) PackageSpec {
  var libs []ComponentSpec
  for _, r := range rules {
    if lib, isLib := existingLibrary(r, sources); isLib { libs = append(libs, lib) }
  }
  return PackageSpec{libs, nil}
}

func AmendRules(args language.GenerateArgs, rules []*rule.Rule, sources Deps, library bool) []RuleResult {
  spec := existingLibraries(rules, sources)
  return multilib(spec, sources, library)
}
