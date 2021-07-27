package okapi

import (
  "encoding/json"
  "flag"
  "fmt"
  "io/ioutil"
  "log"
  "os/exec"
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
// Codept
// -----------------------------------------------------------------------------

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

func depName(file string) string {
  return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
}

func consSource(name string, sigs map[string][]string) Source {
  _, intf := sigs[name]
  return Source{name, intf}
}

func consDeps(dir string, codept Codept) Deps {
  local := make(map[string]string)
  sigs := make(map[string][]string)
  mods := make(map[string][]string)
  sources := make(Deps)
  for _, loc := range codept.Local {
    for _, mod := range loc.Module { local[mod] = depName(loc.Ml) }
  }
  for _, src := range codept.Dependencies {
    if filepath.Dir(src.File) == dir {
      var deps []string
      for _, ds := range src.Deps {
        for _, d := range ds {
          if local[d] != "" { deps = append(deps, local[d]) }
        }
      }
      name := depName(src.File)
      if filepath.Ext(src.File) == ".mli" { sigs[name] = deps } else { mods[name] = deps }
    }
  }
  for src, deps := range mods { sources[consSource(src, sigs)] = deps }
  return sources
}

// While codept is able to scan a directory, there's no way to exclude subdirectories, so files have to be specified
// explicitly.
func runCodept(dir string, files []string) []byte {
  var args = []string{"-native", "-deps"}
  for _, file := range files {
    if filepath.Ext(file) == ".ml" || filepath.Ext(file) == ".mli" {
      args = append(args, dir + "/" + file)
    }
  }
  cmd := exec.Command("codept", args...)
  out, err := cmd.Output()
  if err != nil { log.Fatal("codept failed for " + dir + ": " + string(out[:])) }
  return out
}

func Dependencies(dir string, files []string) Deps {
  out := runCodept(dir, files)
  var codept Codept
  err := json.Unmarshal(out, &codept)
  if err != nil { log.Fatal("Parsing codept output for " + dir + ":\n" + err.Error() + "\n" + string(out[:])) }
  return consDeps(dir, codept)
}

// -----------------------------------------------------------------------------
// Sexp
// -----------------------------------------------------------------------------

type SexpError struct { msg string }
func (e SexpError) Error() string { return e.msg }

type SexpNode interface {
  List() ([]SexpNode, error)
  String() (string, error)
}

type SexpMap struct {
  Name string
  Values map[string]SexpNode
}

func (m SexpMap) List() ([]SexpNode, error) {
  return nil, SexpError{fmt.Sprintf("SexpMap %#v cannot be converted to list", m)}
}

func (m SexpMap) String() (string, error) {
  return "", SexpError{fmt.Sprintf("SexpMap %#v cannot be converted to string", m)}
}

type SexpList struct { Sub []SexpNode }

func (l SexpList) List() ([]SexpNode, error) { return l.Sub, nil }

func (l SexpList) String() (string, error) {
  if len(l.Sub) == 1 {
    return l.Sub[0].String()
  } else {
    return "", SexpError{fmt.Sprintf("SexpList %#v has multiple values", l)}
  }
}

type SexpString struct { Content string }

func (s SexpString) List() ([]SexpNode, error) {
  return []SexpNode{s}, nil
}

func (s SexpString) String() (string, error) {
  return s.Content, nil
}

type SexpEmpty struct {}
func (s SexpEmpty) List() ([]SexpNode, error) { return nil, SexpError{"SexpEmpty cannot be converted to list"} }
func (s SexpEmpty) String() (string, error) { return "", SexpError{"SexpEmpty cannot be converted to string"} }

func sexpStrings(node SexpNode) ([]string, error) {
  var result []string
  l, err := node.List()
  if err != nil { return nil, err }
  for _, el := range l {
    s, err := el.String()
    if err != nil { return nil, SexpError{fmt.Sprintf("Element in SexpList %#v is not a string: %#v", l, el)} }
    result = append(result, s)
  }
  return result, nil
}

func sexpMap(elements []SexpNode) SexpNode {
  if len(elements) > 2 {
    canMap := false
    smap := make(map[string]SexpNode)
    name, nameIsString := elements[0].(SexpString)
    if nameIsString {
      canMap = true
      for _, node := range elements[1:] {
        l, isList := node.(SexpList)
        if isList && len(l.Sub) > 1 {
          s, isString := l.Sub[0].(SexpString)
          if isString && smap[s.Content] == nil {
            var value SexpNode
            if len(l.Sub) == 2 { value = l.Sub[1] } else { value = SexpList{l.Sub[1:]} }
            smap[s.Content] = value
          } else { canMap = false }
        } else { canMap = false }
      }
    }
    if canMap { return SexpMap{name.Content, smap} }
  }
  return SexpList{elements}
}

func sexp(tokens []string) (SexpNode, []string) {
  if len(tokens) > 0 {
    head := tokens[0]
    tail := tokens[1:]
    if head == "(" {
      sub, rest := sexpList(tail)
      return SexpList{sub}, rest
    } else if head == ")" {
      return SexpEmpty {}, tail
    } else { return SexpString{head}, tail }
  } else { return SexpEmpty{}, []string{} }
}

func sexpList(tokens []string) ([]SexpNode, []string) {
  done := false
  cur := tokens
  var result []SexpNode
  for !done {
    if len(cur) > 0 {
      if cur[0] == ")" {
        done = true
        cur = cur[1:]
      } else {
        next, rest := sexp(cur)
        result = append(result, next)
        cur = rest
      }
    } else { done = true }
  }
  return result, cur
}

// -----------------------------------------------------------------------------
// Dune
// -----------------------------------------------------------------------------

func parseDune(duneFile string) SexpList {
  bytes, _ := ioutil.ReadFile(duneFile)
  code := string(bytes[:])
  var tokens []string
  rex := regexp.MustCompile(`\(|\)|\s+|[^()\s]+`)
  ws := regexp.MustCompile(`^\s+$`)
  comment := regexp.MustCompile(`;.*\n`)
  withoutComments := comment.ReplaceAllString(code, "")
  for _, match := range rex.FindAllString(withoutComments, -1) {
    if !ws.MatchString(match) {
      tokens = append(tokens, match)
    }
  }
  items, rest := sexpList(tokens)
  if len(rest) > 0 { log.Fatalf("leftover tokens after parsing sexps: %#v", rest) }
  var result []SexpNode
  for _, node := range items {
    l, isList := node.(SexpList)
    if isList { result = append(result, sexpMap(l.Sub)) } else {
      log.Fatalf("top level dune item is not a list: %#v", node)
    }
  }
  return SexpList{result}
}

type DuneAlt struct {
  Cond string
  Choice string
}

type DuneLibDep interface {}
type DuneLibOpam struct { Name string }
type DuneLibSelect struct {
  Out string
  Alts []DuneAlt
}

type DuneLib struct {
  Name string
  Modules []string
  Flags []string
  Libraries []DuneLibDep
  Auto bool
  Wrapped bool
}

func DuneList(name string, attr string, dune SexpMap) []string {
  var result []string
  raw := dune.Values[attr]
  if raw != nil {
    items, err := sexpStrings(raw)
    if err != nil { log.Fatalf("dune library %s: attr " + attr + " is not a list of strings: %#v", name, raw) }
    // TODO this is an exclude directive
    // if item[:2] == []string{":standard", "\\"}
    for _, item := range items {
      if item[:1] != ":" {
        result = append(result, item)
      }
    }
  }
  return result
}

func duneLibraryDeps(libName string, dune SexpMap) []DuneLibDep {
  var deps []DuneLibDep
  raw := dune.Values["libraries"]
  selectString := SexpString{"select"}
  if raw != nil {
    entries, err := raw.List()
    if err != nil { log.Fatalf("library %s: invalid libraries field: %#v; %s", libName, raw, err) }
    for _, entry := range entries {
      s, err := entry.String()
      if err == nil {
        deps = append(deps, DuneLibOpam{s})
      } else {
        sel, err := entry.List()
        if err != nil { log.Fatalf("library %s: unparsable libraries entry: %#v; %s", libName, sel, err) }
        if len(sel) > 3 && sel[0] == selectString {
          var alts []DuneAlt
          for _, alt := range sel[3:] {
            ss, err := sexpStrings(alt)
            if err == nil && len(ss) == 2 && ss[0] == "->" { alts = append(alts, DuneAlt{"", ss[1]}) } else
            if err == nil && len(ss) == 3 && ss[1] == "->" { alts = append(alts, DuneAlt{ss[0], ss[2]}) } else {
              log.Fatalf("library %s: unparsable select alternative: %#v; %s", libName, alt, err)
            }
          }
          final, err := sel[1].String()
          if err != nil { log.Fatalf("library %s: invalid type for select file name: %#v; %s", libName, sel, err) }
          deps = append(deps, DuneLibSelect{final, alts})
        }
      }
    }
  }
  return deps
}

func DecodeDuneConfig(libName string, conf SexpList) []DuneLib {
  var libraries []DuneLib
  for _, node := range conf.Sub {
    dune, isMap := node.(SexpMap)
    if isMap {
      if dune.Name == "library" {
        name, nameIsString := dune.Values["name"].(SexpString)
        if !nameIsString { log.Fatalf("dune library %s: name isn't a string: %#v", libName, dune.Values["name"]) }
        wrapped := dune.Values["wrapped"] != SexpString{"false"}
        modules := DuneList(libName, "modules", dune)
        lib := DuneLib{
          Name: name.Content,
          Modules: modules,
          Flags: DuneList(libName, "flags", dune),
          Libraries: duneLibraryDeps(libName, dune),
          Auto: len(modules) == 0,
          Wrapped: wrapped,
        }
        libraries = append(libraries, lib)
      }
    }
  }
  return libraries
}

func contains(target string, items []string) bool {
  for _, item := range items {
    if target == item {return true}
  }
  return false
}

func filterSelects(modules []string, libs []DuneLibDep) []string {
  var result []string
  var alts []string
  for _, lib := range libs {
    sel, isSel := lib.(DuneLibSelect)
    if isSel {
      result = append(result, depName(sel.Out))
      for _, alt := range sel.Alts {
        alts = append(alts, depName(alt.Choice))
      }
    }
  }
  for _, lib := range modules {
    if !contains(lib, alts) { result = append(result, lib) }
  }
  return result
}

func opamDeps(deps []DuneLibDep) []string {
  var result []string
  for _, dep := range deps {
    ld, isOpam := dep.(DuneLibOpam)
    if isOpam { result = append(result, ld.Name) }
  }
  return result
}

func duneAttrs(rule *rule.Rule, dune DuneLib) *rule.Rule {
  rule.SetAttr("deps_opam", opamDeps(dune.Libraries))
  rule.SetAttr("opts", dune.Flags)
  return rule
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
  sort.Strings(deps)
  return deps
}

func libraryRuleFor(name string, modules []string, auto bool) *rule.Rule {
  r := rule.NewRule("ocaml_ns_library", name)
  r.SetAttr("visibility", []string{"//visibility:public"})
  r.SetAttr("submodules", modules)
  if auto { r.AddComment("# okapi:auto") }
  return r
}

func libraryRule(name string, sources Deps) *rule.Rule {
  return libraryRuleFor(name, libraryDeps(sources), false)
}

func resultForRules(rules []*rule.Rule) language.GenerateResult {
  var imports []interface{}
  for range rules { imports = append(imports, 0) }
  return language.GenerateResult{
    Gen: rules,
    Empty: []*rule.Rule{},
    Imports: imports,
  }
}

func sigModRules(src Source, deps []string) []*rule.Rule {
  sort.Strings(deps)
  var rules []*rule.Rule
  rules = append(rules, moduleRule(src, deps))
  if src.Intf { rules = append(rules, signatureRule(src, deps)) }
  return rules
}

func sourceRules(names []string, sources Deps) []*rule.Rule {
  var rules []*rule.Rule
  log.Print(names)
  sort.Strings(names)
  log.Print(names)
  for _, name := range names {
    for src, deps := range sources {
      if src.Name == name[1:] {
        rules = append(rules, sigModRules(src, deps)...)
      }
    }
  }
  return rules
}

func sourceRulesAuto(sources Deps) []*rule.Rule {
  var rules []*rule.Rule
  var keys []Source
  for key := range sources { keys = append(keys, key) }
  sort.Slice(keys, func(i, j int) bool { return keys[i].Name < keys[j].Name })
  for _, src := range keys {
    deps := sources[src]
    rules = append(rules, sigModRules(src, deps)...)
  }
  return rules
}

func GenerateRulesAuto(name string, sources Deps) []*rule.Rule {
  return append(sourceRulesAuto(sources), libraryRule(name, sources))
}

func duneLib(sources Deps, dune DuneLib) []*rule.Rule {
  byName := make(map[string]Source)
  for src := range sources { byName[src.Name] = src }
  var rules []*rule.Rule
  modules := filterSelects(dune.Modules, dune.Libraries)
  for _, name := range modules {
    src, srcExists := byName[name]
    if !srcExists { src = Source{name, false} }
    deps := sources[src]
    rules = append(rules, duneAttrs(moduleRule(src, deps), dune))
    if src.Intf { rules = append(rules, duneAttrs(signatureRule(src, deps), dune)) }
  }
  return append(rules, libraryRuleFor(generateLibraryName(dune.Name), modules, dune.Auto))
}

func singleDuneLib(sources Deps, dune DuneLib) []*rule.Rule {
  var modules []string
  for src := range sources { modules = append(modules, src.Name) }
  dune.Modules = modules
  return duneLib(sources, dune)
}

func duneLibsWithAuto(sources Deps, auto DuneLib, concrete []DuneLib) []*rule.Rule {
  var autoModules []string
  for src := range sources {
    auto := true
    for _, lib := range concrete {
      for _, mod := range lib.Modules {
        if mod == src.Name { auto = false }
      }
    }
    if auto { autoModules = append(autoModules, src.Name) }
  }
  auto.Modules = autoModules
  rules := duneLib(sources, auto)
  for _, lib := range concrete { rules = append(rules, duneLib(sources, lib)...) }
  return rules
}

func duneLibsWithoutAuto(sources Deps, libs []DuneLib) []*rule.Rule {
  return nil
}

func GenerateRulesDune(name string, sources Deps, duneCode string) []*rule.Rule {
  conf := parseDune(duneCode)
  libs := DecodeDuneConfig(name, conf)
  if len(libs) == 0 { return nil }
  if len(libs) == 1 { return singleDuneLib(sources, libs[0]) }
  hasAutoLib := false
  var autoLib DuneLib
  var concreteLibs []DuneLib
  for _, lib := range libs {
    if len(lib.Modules) == 0 {
      hasAutoLib = true
      autoLib = lib
    } else {
      concreteLibs = append(concreteLibs, lib)
    }
  }
  if hasAutoLib {
    return duneLibsWithAuto(sources, autoLib, concreteLibs)
  } else {
    return duneLibsWithoutAuto(sources, libs)
  }
}

func GenerateRules(name string, sources Deps, dune string) []*rule.Rule {
  if dune == "" { return GenerateRulesAuto(name, sources) } else { return GenerateRulesDune(name, sources, dune) }
}

var EmptyResult = language.GenerateResult{
  Gen: []*rule.Rule{},
  Empty: []*rule.Rule{},
  Imports: []interface{}{},
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

func findAutoLib(libs []*rule.Rule) (*rule.Rule, []*rule.Rule) {
  var auto *rule.Rule
  var nonAuto []*rule.Rule
  for _, lib := range libs {
    if contains("auto", tags(lib)) { auto = lib } else { nonAuto = append(nonAuto, lib) }
  }
  return auto, nonAuto
}

func multilibWithoutAuto(libs []*rule.Rule, sources Deps) []*rule.Rule {
  var rules []*rule.Rule
  for _, r := range libs {
    sub := r.AttrStrings("submodules")
    rules = append(rules, sourceRules(sub, sources)...)
  }
  return rules
}

func multilibWithAuto(auto *rule.Rule, nonAuto []*rule.Rule, sources Deps) []*rule.Rule {
  var rules []*rule.Rule
  var nonAutoModules map[string]bool
  for _, r := range nonAuto {
    sub := r.AttrStrings("submodules")
    rules = append(rules, sourceRules(sub, sources)...)
    for _, mod := range sub {
      nonAutoModules[mod[1:]] = true
    }
  }
  var autoSources Deps
  for src, deps := range sources {
    if !nonAutoModules[src.Name] { autoSources[src] = deps }
  }
  return append(rules, sourceRulesAuto(autoSources)...)
}

// Update an existing build that has been manually amended by the user to contain more than one library.
// In that case, all submodule assignments are static, and only the module/signature rules are updated.
func Multilib(libs []*rule.Rule, sources Deps) []*rule.Rule {
  auto, nonAuto := findAutoLib(libs)
  if auto == nil {
    return multilibWithoutAuto(libs, sources)
  } else {
    return multilibWithAuto(auto, nonAuto, sources)
  }
}

func AmendRules(args language.GenerateArgs, rules []*rule.Rule, sources Deps) []*rule.Rule {
  var libs []*rule.Rule
  for _, r := range rules {
    if r.Kind() == "ocaml_ns_library" { libs = append(libs, r) }
  }
  if len(libs) == 1 {
    return GenerateRulesAuto(libs[0].Name(), sources)
  } else if len(libs) > 1 {
    return Multilib(libs, sources)
  } else {
    return nil
  }
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

type Config struct { }

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
  MergeableAttrs: map[string]bool{"submodules": true},
  ResolveAttrs: map[string]bool{},
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
      Name: "@obazl_rules_ocaml//ocaml:rules.bzl",
      Symbols: []string{"ocaml_ns_library", "ocaml_module", "ocaml_signature"},
      After: []string{},
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

func containsLibrary(rules []*rule.Rule) bool {
  for _, r := range rules {
    if r.Kind() == "ocaml_ns_library" {
      return true
    }
  }
  return false
}

func findDune(dir string, files []string) string {
  for _, f := range files {
    if f == "dune" { return filepath.Join(dir, f) }
  }
  return ""
}

func (*okapiLang) GenerateRules(args language.GenerateArgs) language.GenerateResult {
  var rules []*rule.Rule
  var imports []interface{}
  if args.File != nil && args.File.Rules != nil && containsLibrary(args.File.Rules) {
    rules = AmendRules(args, args.File.Rules, Dependencies(args.Dir, args.RegularFiles))
  } else {
    for _, file := range args.RegularFiles {
      ext := filepath.Ext(file)
      if ext == ".ml" || ext == ".mli" {
        rules = GenerateRules(
          generateLibraryName(args.Dir),
          Dependencies(args.Dir, args.RegularFiles),
          findDune(args.Dir, args.RegularFiles),
        )
      }
    }
  }
  for range rules { imports = append(imports, 0) }
  return language.GenerateResult{
    Gen: rules,
    Empty: []*rule.Rule{},
    Imports: imports,
  }
}
