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
  Deps []string
}

type Deps = map[string]Source

func depName(file string) string {
  return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
}

func consSource(name string, sigs map[string][]string, deps []string) Source {
  _, intf := sigs[name]
  return Source{
    Name: name,
    Intf: intf,
    Deps: deps,
  }
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
  for src, deps := range mods { sources[src] = consSource(src, sigs, deps) }
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
  if len(l) == 1 {
    if singleton, isSingleton := l[0].(SexpList); isSingleton {
      l = singleton.Sub
    }
  }
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
        if isList && len(l.Sub) >= 1 {
          s, isString := l.Sub[0].(SexpString)
          if isString && smap[s.Content] == nil {
            var value SexpNode
            if len(l.Sub) == 2{
              s, sErr := l.Sub[1].String()
              if sErr == nil {
                value = SexpString{s}
              } else {
                value = SexpList{l.Sub[1:]}
              }
            } else if len(l.Sub) == 1 {
              value = SexpEmpty{}
            } else {
              value = SexpList{l.Sub[1:]}
            }
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
// Dune
// -----------------------------------------------------------------------------

type DuneLibDep interface {}
type DuneLibOpam struct { Name string }
type DuneLibSelect struct {
  Choice ModuleChoice
}

type DuneLib struct {
  Name string
  Modules []string
  Flags []string
  Libraries []DuneLibDep
  Auto bool
  Wrapped bool
  Ppx bool
  Preprocess []string
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

func DuneList(name string, attr string, dune SexpMap) []string {
  var result []string
  raw := dune.Values[attr]
  if raw != nil {
    items, err := sexpStrings(raw)
    if err != nil { log.Fatalf("dune library %s: attr " + attr + " is not a list of strings: %s: %#v", name, err, raw) }
    // TODO this is an exclude directive
    // if item[:2] == []string{":standard", "\\"}
    for _, item := range items {
      if item[:1] != ":" { result = append(result, item) }
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
          var alts []ModuleAlt
          for _, alt := range sel[3:] {
            ss, err := sexpStrings(alt)
            if err == nil && len(ss) == 2 && ss[0] == "->" { alts = append(alts, ModuleAlt{"", ss[1]}) } else
            if err == nil && len(ss) == 3 && ss[1] == "->" { alts = append(alts, ModuleAlt{ss[0], ss[2]}) } else {
              log.Fatalf("library %s: unparsable select alternative: %#v; %s", libName, alt, err)
            }
          }
          final, err := sel[1].String()
          if err != nil { log.Fatalf("library %s: invalid type for select file name: %#v; %s", libName, sel, err) }
          deps = append(deps, DuneLibSelect{ModuleChoice{final, alts}})
        }
      }
    }
  }
  return deps
}

func dunePreprocessors(libName string, dune SexpMap) []string {
  var result []string
  raw := dune.Values["preprocess"]
  if raw != nil {
    if items, err := raw.List(); err == nil {
      for _, item := range items {
        elems, err := item.List()
        pps := SexpString{"pps"}
        if err == nil && len(elems) == 2 && elems[0] == pps {
          pp, stringErr := elems[1].String()
          if stringErr != nil { log.Fatalf("dune library %s: pps is not a string: %#v", libName, elems[1]) }
          result = append(result, pp)
        }
      }
    } else {
      log.Printf("dune library %s: Warning: invalid `preprocess` directive: %#v", libName, raw)
    }
  }
  return result
}

func DecodeDuneConfig(libName string, conf SexpList) []DuneLib {
  var libraries []DuneLib
  for _, node := range conf.Sub {
    dune, isMap := node.(SexpMap)
    if isMap && dune.Name == "library" {
      nameRaw, nameRawErr := dune.Values["name"]
      if !nameRawErr { log.Fatalf("dune library %s: no name attribute", libName) }
      name, nameErr := nameRaw.String()
      if nameErr != nil { log.Fatalf("dune library %s: name isn't a string: %#v", libName, dune.Values["name"]) }
      wrapped := dune.Values["wrapped"] != SexpString{"false"}
      modules := DuneList(libName, "modules", dune)
      preproc := dunePreprocessors(libName, dune)
      lib := DuneLib{
        Name: name,
        Modules: modules,
        Flags: DuneList(libName, "flags", dune),
        Libraries: duneLibraryDeps(libName, dune),
        Auto: len(modules) == 0,
        Wrapped: wrapped,
        Ppx: len(preproc) > 0,
        Preprocess: preproc,
      }
      libraries = append(libraries, lib)
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

func modulesWithSelectOutputs(modules []string, libs []DuneLibDep) []string {
  var result []string
  var alts []string
  for _, lib := range libs {
    if sel, isSel := lib.(DuneLibSelect); isSel {
      result = append(result, depName(sel.Choice.Out))
      for _, alt := range sel.Choice.Alts { alts = append(alts, depName(alt.Choice)) }
    }
  }
  for _, lib := range modules { if !contains(lib, alts) { result = append(result, lib) } }
  return result
}

func duneChoices(libs []DuneLibDep) []ModuleChoice {
  var choices []ModuleChoice
  for _, dep := range libs {
    if sel, isSel := dep.(DuneLibSelect); isSel { choices = append(choices, sel.Choice) }
  }
  return choices
}

func opamDeps(deps []DuneLibDep) []string {
  var result []string
  for _, dep := range deps {
    ld, isOpam := dep.(DuneLibOpam)
    if isOpam { result = append(result, ld.Name) }
  }
  return result
}

func dunePpx(deps []string, wrapped bool) Kind {
  ppx := false
  if len(deps) > 0 { ppx = true }
  if wrapped {
    if ppx { return KindNsPpx{PpxDirect{deps}} } else { return KindNs{} }
  } else {
    if ppx { return KindPpx{PpxDirect{deps}} } else { return KindPlain{} }
  }
}

func duneToLibrary(dune DuneLib) Library {
  return Library{
    Slug: dune.Name,
    Name: generateLibraryName(dune.Name),
    Modules: modulesWithSelectOutputs(dune.Modules, dune.Libraries),
    Opts: dune.Flags,
    DepsOpam: opamDeps(dune.Libraries),
    Choices: duneChoices(dune.Libraries),
    Auto: dune.Auto,
    Wrapped: dune.Wrapped,
    Kind: dunePpx(dune.Preprocess, dune.Wrapped),
  }
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
  conf := parseDune(duneCode)
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

func findDune(dir string, files []string) string {
  for _, f := range files {
    if f == "dune" { return filepath.Join(dir, f) }
  }
  return ""
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
