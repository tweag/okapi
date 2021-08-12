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

type DuneKind interface {}

type DuneLib struct {
  wrapped bool
  virtualModules []string
  implements string
}

type DuneExe struct {
}

type DuneGenerated interface {
  convert() Generated
}

type DuneLex struct {
  name string
}

func (lex DuneLex) convert() Generated {
  return Lex{lex.name}
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
  generated []DuneGenerated
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

func generatedSources(conf SexpList) []DuneGenerated {
  var result []DuneGenerated
  for _, node := range conf.Sub {
    if l, err := node.List(); err == nil {
      if len(l) == 2 && (l[0] == SexpString{"ocamllex"}) {
        if lex, err := l[1].String(); err == nil {
          result = append(result, DuneLex{lex})
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

func duneKindToOBazl(dune DuneComponent, ppx PpxKind) ComponentKind {
  if lib, isLib := dune.kind.(DuneLib); isLib {
    return Library{
      virtualModules: lib.virtualModules,
      implements: lib.implements,
      kind: libKind(ppx.isPpx(), lib.wrapped),
    }
  } else {
    var kind ExeKind = ExePlain{}
    if ppx.isPpx() { kind = ExePpx{} }
    return Executable{
      kind: kind,
    }
  }
}

func assignDuneGenerated(conf DuneConfig) map[string][]Generated {
  byGen := make(map[Generated]string)
  result := make(map[string][]Generated)
  var gens []Generated
  for _, gen := range conf.generated { gens = append(gens, gen.convert()) }
  for _, gen := range gens {
    if gen.moduleDep() {
      for _, com := range conf.components {
        _, exists := byGen[gen]
        if com.auto {
          if !exists { byGen[gen] = com.name }
        } else {
          if contains(gen.target(), com.modules) { byGen[gen] = com.name }
        }
      }
    }
  }
  for gen, lib := range byGen {
    val := []Generated{gen}
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

func duneToOBazl(dune DuneComponent, generated map[string][]Generated) Component {
  ppx := dunePpx(dune.preprocess)
  return Component{
    name: dune.name,
    publicName: dune.publicName,
    modules: modulesWithSelectOutputs(dune.modules, dune.libraries),
    opts: dune.flags,
    depsOpam: opamDeps(dune.libraries),
    choices: duneChoices(dune.libraries),
    auto: dune.auto,
    ppx: ppx,
    generated: generated[dune.name],
    kind: duneKindToOBazl(dune, ppx),
  }
}

func findDune(dir string, files []string) string {
  for _, f := range files {
    if f == "dune" { return filepath.Join(dir, f) }
  }
  return ""
}
