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
// Dune
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

func sexpGroup(elements []SexpNode) SexpNode {
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
      return sub, rest
    } else if head == ")" {
      return SexpEmpty {}, tail
    } else { return SexpString{head}, tail }
  } else { return SexpEmpty{}, []string{} }
}

func sexpList(tokens []string) (SexpNode, []string) {
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
  return sexpGroup(result), cur
}

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
  result, rest := sexpList(tokens)
  if len(rest) > 0 { log.Fatalf("leftover tokens after parsing sexps: %#v", rest) }
  list, isList := result.(SexpList)
  if !isList { log.Fatalf("dune parsing didn't produce a list: %#v'", result) }
  return list
}

type DuneLib struct {
  Name string
  Modules []string
  Flags []string
  Libraries []string
}

func DuneList(name string, attr string, dune SexpMap) []string {
  var result []string
  raw := dune.Values[attr]
  if raw != nil {
    items, err := sexpStrings(raw)
    if err != nil { log.Fatalf("dune library %s: attr " + attr + " is not a list: %#v", name, raw) }
    for _, item := range items {
      if item[:1] != ":" {
        result = append(result, item)
      }
    }
  }
  return result
}

func duneLibraryDeps(libName string, dune SexpMap) []string {
  var mods []string
  raw := dune.Values["libraries"]
  selectString := SexpString{"select"}
  if raw != nil {
    entries, err := raw.List()
    if err != nil { log.Fatalf("library %s: invalid libraries field: %#v; %s", libName, raw, err) }
    for _, entry := range entries {
      s, err := entry.String()
      if err == nil {
        mods = append(mods, s)
      } else {
        sel, err := entry.List()
        if err != nil { log.Fatalf("library %s: unparsable libraries entry: %#v; %s", libName, sel, err) }
        if len(sel) > 3 && sel[0] == selectString {
          for _, alt := range sel[3:] {
            ss, err := sexpStrings(alt)
            if err != nil || len(ss) < 2 {
              log.Fatalf("library %s: unparsable select alternative: %#v; %s", libName, alt, err)
            }
            if ss[0] == "->" {
              mods = append(mods, ss[1])
            }
          }
        }
      }
    }
  }
  return mods
}

func DecodeDuneConfig(libName string, conf SexpList) []DuneLib {
  var libraries []DuneLib
  for _, node := range conf.Sub {
    dune, isMap := node.(SexpMap)
    if isMap {
      if dune.Name == "library" {
        name, nameIsString := dune.Values["name"].(SexpString)
        if !nameIsString { log.Fatalf("dune library %s: name isn't a string: %#v", libName, dune.Values["name"]) }
        lib := DuneLib{
          Name: name.Content,
          Modules: DuneList(libName, "modules", dune),
          Flags: DuneList(libName, "flags", dune),
          Libraries: duneLibraryDeps(libName, dune),
        }
        libraries = append(libraries, lib)
      }
    }
  }
  return libraries
}

func duneAttrs(rule *rule.Rule, dune DuneLib) *rule.Rule {
  rule.SetAttr("deps_opam", dune.Libraries)
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
  return deps
}

func libraryRuleFor(name string, modules []string) *rule.Rule {
  r := rule.NewRule("ocaml_ns_library", name)
  r.SetAttr("visibility", []string{"//visibility:public"})
  r.SetAttr("submodules", modules)
  return r
}

func libraryRule(name string, sources Deps) *rule.Rule {
  return libraryRuleFor(name, libraryDeps(sources))
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

func GenerateRulesAuto(name string, sources Deps) []*rule.Rule {
  var rules []*rule.Rule
  for src, deps := range sources {
    rules = append(rules, moduleRule(src, deps))
    if src.Intf { rules = append(rules, signatureRule(src, deps)) }
  }
  return append(rules, libraryRule(name, sources))
}

func duneLib(sources Deps, dune DuneLib) []*rule.Rule {
  byName := make(map[string]Source)
  for src := range sources { byName[src.Name] = src }
  var rules []*rule.Rule
  for _, name := range dune.Modules {
    src := byName[name]
    deps := sources[src]
    rules = append(rules, duneAttrs(moduleRule(src, deps), dune))
    if src.Intf { rules = append(rules, duneAttrs(signatureRule(src, deps), dune)) }
  }
  return append(rules, libraryRuleFor(generateLibraryName(dune.Name), dune.Modules))
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

func GenerateRules(name string, sources Deps, dune string) language.GenerateResult {
  var rules []*rule.Rule
  if dune == "" { rules = GenerateRulesAuto(name, sources) } else { rules = GenerateRulesDune(name, sources, dune) }
  return resultForRules(rules)
}

func sourceRules(names []string, sources Deps) []*rule.Rule {
  var rules []*rule.Rule
  for _, name := range names {
    for src, deps := range sources {
      if src.Name == name[1:] {
        rules = append(rules, moduleRule(src, deps))
        if src.Intf { rules = append(rules, signatureRule(src, deps)) }
      }
    }
  }
  return rules
}

var EmptyResult = language.GenerateResult{
  Gen: []*rule.Rule{},
  Empty: []*rule.Rule{},
  Imports: []interface{}{},
}

// Update an existing build that has been manually amended by the user to contain more than one library.
// In that case, all submodule assignments are static, and only the module/signature rules are updated.
// TODO allow the user to mark one of the libraries as `auto`, causing new files to be added to it.
func Multilib(libs []*rule.Rule, sources Deps) language.GenerateResult {
  var rules []*rule.Rule
  var imports []interface{}
  for _, r := range libs {
    sub := r.AttrStrings("submodules")
    rules = append(rules, sourceRules(sub, sources)...)
  }
  for range rules { imports = append(imports, 0) }
  return language.GenerateResult{
    Gen: rules,
    Empty: []*rule.Rule{},
    Imports: imports,
  }
}

func AmendRules(args language.GenerateArgs, rules []*rule.Rule, sources Deps) language.GenerateResult {
  var libs []*rule.Rule
  for _, r := range rules {
    if r.Kind() == "ocaml_ns_library" { libs = append(libs, r) }
  }
  if len(libs) == 1 {
    return GenerateRules(libs[0].Name(), sources, "")
  } else if len(libs) > 1 {
    return Multilib(libs, sources)
  } else {
    return EmptyResult
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
  if args.File != nil && args.File.Rules != nil && containsLibrary(args.File.Rules) {
    return AmendRules(args, args.File.Rules, Dependencies(args.Dir, args.RegularFiles))
  }
  for _, file := range args.RegularFiles {
    ext := filepath.Ext(file)
    if ext == ".ml" || ext == ".mli" {
      return GenerateRules(
        generateLibraryName(args.Dir),
        Dependencies(args.Dir, args.RegularFiles),
        findDune(args.Dir, args.RegularFiles),
      )
    }
  }
  return EmptyResult
}
