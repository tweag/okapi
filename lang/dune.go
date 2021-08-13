package okapi

import (
  "fmt"
  "io/ioutil"
  "log"
  "path/filepath"
)

type DuneLibDep interface {}
type DuneLibOpam struct { name string }
type DuneLibSelect struct { Choice ModuleChoice }

type DuneKind interface {
  toObazl(DuneComponent, PpxKind, Deps) ComponentKind
}

type DuneLib struct {
  wrapped bool
  virtualModules []string
  implements string
}

type DuneExe struct { }

func (lib DuneLib) toObazl(component DuneComponent, ppx PpxKind, sources Deps) ComponentKind {
  var modules []Source
  for _, mod := range lib.virtualModules {
    modules = append(modules, sources[mod])
  }
  return Library{
    virtualModules: modules,
    implements: lib.implements,
    kind: libKind(ppx.isPpx(), lib.wrapped),
  }
}

func (DuneExe) toObazl(component DuneComponent, ppx PpxKind, sources Deps) ComponentKind {
  var kind ExeKind = ExePlain{}
  if ppx.isPpx() { kind = ExePpx{} }
  return Executable{
    kind: kind,
  }
}

type DuneComponent struct {
  name string
  publicName string
  modules []string
  flags []string
  libraries []DuneLibDep
  auto bool
  ppx bool
  preprocess []string
  kind DuneKind
}

type DuneConfig struct {
  components []DuneComponent
  generated []string
}

func parseDune(code string) SexpList {
  items := parseSexp(code)
  var result []SexpNode
  for _, node := range items {
    if l, isList := node.(SexpList); isList {
      result = append(result, sexpMap(l.Sub))
    } else {
      log.Fatalf("top level dune item is not a list: %#v", node)
    }
  }
  return SexpList{result}
}

func parseDuneFile(duneFile string) SexpList {
  bytes, _ := ioutil.ReadFile(duneFile)
  code := string(bytes[:])
  return parseDune(code)
}

type SexpLib struct {
  name string
  data SexpMap
}

func (lib SexpLib) fatalf(msg string, v ...interface{}) {
  log.Fatalf(fmt.Sprintf("dune library %s: ", lib.name) + msg, v...)
}

func (lib SexpLib) list(attr string) []string {
  var result []string
  raw := lib.data.Values[attr]
  if raw != nil {
    items, err := sexpStrings(raw)
    if err != nil { lib.fatalf("attr %s is not a list of strings: %s: %#v", attr, err, raw) }
    // TODO this is an exclude directive
    // if item[:2] == []string{":standard", "\\"}
    for _, item := range items {
      if item[:1] != ":" { result = append(result, item) }
    }
  }
  return result
}

func (lib SexpLib) stringOr(key string, def string) string {
  raw, exists := lib.data.Values[key]
  if exists {
    value, stringErr := raw.String()
    if stringErr != nil { lib.fatalf("%s isn't a string: %#v", key, raw) }
    return value
  } else {
    return def
  }
}

func (lib SexpLib) stringOptional(key string) string { return lib.stringOr(key, "") }

func (lib SexpLib) string(key string) string {
  value := lib.stringOptional(key)
  if value == "" { lib.fatalf("no `%s` attribute", key) }
  return value
}

func duneLibraryDeps(lib SexpLib) []DuneLibDep {
  var deps []DuneLibDep
  raw := lib.data.Values["libraries"]
  selectString := SexpString{"select"}
  if raw != nil {
    entries, err := raw.List()
    if err != nil { lib.fatalf("invalid libraries field: %#v; %s", raw, err) }
    for _, entry := range entries {
      s, err := entry.String()
      if err == nil {
        deps = append(deps, DuneLibOpam{s})
      } else {
        sel, err := entry.List()
        if err != nil { lib.fatalf("unparsable libraries entry: %#v; %s", sel, err) }
        if len(sel) > 3 && sel[0] == selectString {
          var alts []ModuleAlt
          for _, alt := range sel[3:] {
            ss, err := sexpStrings(alt)
            if err == nil && len(ss) == 2 && ss[0] == "->" { alts = append(alts, ModuleAlt{"", ss[1]}) } else
            if err == nil && len(ss) == 3 && ss[1] == "->" { alts = append(alts, ModuleAlt{ss[0], ss[2]}) } else {
              lib.fatalf("unparsable select alternative: %#v; %s", alt, err)
            }
          }
          final, err := sel[1].String()
          if err != nil { lib.fatalf("library %s: invalid type for select file name: %#v; %s", sel, err) }
          deps = append(deps, DuneLibSelect{ModuleChoice{final, alts}})
        }
      }
    }
  }
  return deps
}

func dunePreprocessors(lib SexpLib) []string {
  var result []string
  raw := lib.data.Values["preprocess"]
  if raw != nil {
    if items, err := raw.List(); err == nil {
      for _, item := range items {
        elems, err := item.List()
        pps := SexpString{"pps"}
        if err == nil && len(elems) == 2 && elems[0] == pps {
          pp, stringErr := elems[1].String()
          if stringErr != nil { lib.fatalf("dune library %s: pps is not a string: %#v", elems[1]) }
          result = append(result, pp)
        }
      }
    } else {
      lib.fatalf("invalid `preprocess` directive: %#v", raw)
    }
  }
  return result
}

func decodeDuneComponent(lib SexpLib) DuneKind {
  wrapped := lib.data.Values["wrapped"] != SexpString{"false"}
  if lib.data.Name == "library" {
    return DuneLib{
      wrapped: wrapped,
      virtualModules: lib.list("virtual_modules"),
      implements: lib.stringOptional("implements"),
    }
  } else if lib.data.Name == "executable" {
    return DuneExe{}
  }
  return nil
}

func generatedSources(conf SexpList) []string {
  var result []string
  for _, node := range conf.Sub {
    if l, err := node.List(); err == nil {
      if len(l) == 2 && (l[0] == SexpString{"ocamllex"}) {
        if lex, err := l[1].String(); err == nil {
          result = append(result, lex)
        } else {
          log.Fatalf("Invalid name for ocamllex: %#v", l[1])
        }
      }
    }
  }
  return result
}

func DecodeDuneConfig(libName string, conf SexpList) DuneConfig {
  var components []DuneComponent
  generated := generatedSources(conf)
  for _, node := range conf.Sub {
    dune, isMap := node.(SexpMap)
    if isMap && (dune.Name == "library" || dune.Name == "executable") {
      data := SexpLib{libName, dune}
      name := data.string("name")
      publicName := data.stringOr("public_name", name)
      modules := data.list("modules")
      preproc := dunePreprocessors(data)
      auto := len(modules) == 0
      lib := DuneComponent{
        name: name,
        publicName: publicName,
        modules: data.list("modules"),
        flags: data.list("flags"),
        libraries: duneLibraryDeps(data),
        auto: auto,
        ppx: len(preproc) > 0,
        preprocess: preproc,
        kind: decodeDuneComponent(data),
      }
      components = append(components, lib)
    }
  }
  return DuneConfig{components, generated}
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
      result = append(result, depName(sel.Choice.out))
      for _, alt := range sel.Choice.alts { alts = append(alts, depName(alt.choice)) }
    }
  }
  for _, lib := range modules { if !contains(lib, alts) { result = append(result, lib) } }
  return result
}

func duneChoices(libs []DuneLibDep) []Source {
  var choices []Source
  for _, dep := range libs {
    if sel, isSel := dep.(DuneLibSelect); isSel {
      src := Source{depName(sel.Choice.out), false, false, nil, Choice{sel.Choice.alts}}
      choices = append(choices, src)
    }
  }
  return choices
}

func opamDeps(deps []DuneLibDep) []string {
  var result []string
  for _, dep := range deps {
    ld, isOpam := dep.(DuneLibOpam)
    if isOpam { result = append(result, ld.name) }
  }
  return result
}

func dunePpx(deps []string) PpxKind {
  if len(deps) > 0 {
    return PpxDirect{deps}
  } else {
    return NoPpx{}
  }
}

func libKind(ppx bool, wrapped bool) LibraryKind {
  if ppx {
    if wrapped { return LibNsPpx{} } else { return LibPpx{} }
  } else {
    if wrapped { return LibNs{} } else { return LibPlain{} }
  }
}

func assignDuneGenerated(conf DuneConfig) map[string][]string {
  byGen := make(map[string]string)
  result := make(map[string][]string)
  var gens []string
  for _, gen := range conf.generated { gens = append(gens, gen) }
  for _, gen := range gens {
    for _, com := range conf.components {
      _, exists := byGen[gen]
      if com.auto {
        if !exists { byGen[gen] = com.name }
      } else {
        if contains(gen, com.modules) { byGen[gen] = com.name }
      }
    }
  }
  for gen, lib := range byGen {
    val := []string{gen}
    if cur, exists := result[lib]; exists {
      val = append(val, cur...)
    }
    result[lib] = val
  }
  for _, gen := range gens {
    if _, exists := byGen[gen]; !exists {
      log.Fatalf("Could not assign generator: %#v", gen)
    }
  }
  return result
}

func isChoice(name string, choices []Source) bool {
  for _, c := range choices {
    if c.Name == name { return true }
  }
  return false
}

func moduleSources(names []string, sources Deps, choices []Source) []Source {
  var result []Source
  for _, name := range names {
    if src, exists := sources[name]; exists {
      result = append(result, src)
    } else if !isChoice(name, choices) {
      log.Fatalf("Library refers to unknown source `%s`.", name)
    }
  }
  final := append(result, choices...)
  sortSources(final)
  return final
}

func duneToOBazl(dune DuneComponent, generated map[string][]string, sources Deps) Component {
  ppx := dunePpx(dune.preprocess)
  choices := duneChoices(dune.libraries)
  moduleNames := modulesWithSelectOutputs(dune.modules, dune.libraries)
  return Component{
    name: dune.name,
    publicName: dune.publicName,
    modules: moduleSources(append(moduleNames, generated[dune.name]...), sources, choices),
    opts: dune.flags,
    depsOpam: opamDeps(dune.libraries),
    auto: dune.auto,
    ppx: ppx,
    kind: dune.kind.toObazl(dune, ppx, sources),
  }
}

func findDune(dir string, files []string) string {
  for _, f := range files {
    if f == "dune" { return filepath.Join(dir, f) }
  }
  return ""
}
