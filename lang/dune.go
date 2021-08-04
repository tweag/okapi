package okapi

import (
  "fmt"
  "io/ioutil"
  "log"
  "path/filepath"
)

type DuneLibDep interface {}
type DuneLibOpam struct { Name string }
type DuneLibSelect struct { Choice ModuleChoice }

type DuneLib struct {
  Name string
  PublicName string
  Modules []string
  Flags []string
  Libraries []DuneLibDep
  Auto bool
  Wrapped bool
  Ppx bool
  Preprocess []string
  Implements string
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

func DecodeDuneConfig(libName string, conf SexpList) []DuneLib {
  var libraries []DuneLib
  for _, node := range conf.Sub {
    dune, isMap := node.(SexpMap)
    if isMap && dune.Name == "library" {
      data := SexpLib{libName, dune}
      name := data.string("name")
      publicName := data.stringOr("public_name", name)
      wrapped := dune.Values["wrapped"] != SexpString{"false"}
      modules := data.list("modules")
      preproc := dunePreprocessors(data)
      lib := DuneLib{
        Name: name,
        PublicName: publicName,
        Modules: modules,
        Flags: data.list("flags"),
        Libraries: duneLibraryDeps(data),
        Auto: len(modules) == 0,
        Wrapped: wrapped,
        Ppx: len(preproc) > 0,
        Preprocess: preproc,
        Implements: data.stringOptional("implements"),
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
    PublicName: dune.PublicName,
    Implements: dune.Implements,
    Modules: modulesWithSelectOutputs(dune.Modules, dune.Libraries),
    Opts: dune.Flags,
    DepsOpam: opamDeps(dune.Libraries),
    Choices: duneChoices(dune.Libraries),
    Auto: dune.Auto,
    Wrapped: dune.Wrapped,
    Kind: dunePpx(dune.Preprocess, dune.Wrapped),
  }
}

func findDune(dir string, files []string) string {
  for _, f := range files {
    if f == "dune" { return filepath.Join(dir, f) }
  }
  return ""
}
