package okapi

import (
  "encoding/json"
  "log"
  "os/exec"
  "path/filepath"
  "strings"
)

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
  Virtual bool
  Deps []string
}

type Deps = map[string]Source

func depName(file string) string {
  return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
}

func consSource(name string, intfs map[string][]string, deps []string) Source {
  intf, hasIntf := intfs[name]
  return Source{
    Name: name,
    Intf: hasIntf,
    Virtual: false,
    Deps: append(deps, intf...),
  }
}

func modulePath(segments []string) string { return strings.Join(segments, ".") }

// The `local` key in the codept output maps all used modules to their defining source files with the structure
// { "module": ["Qualified", "Module", "Name"], "ml": "/path/to/name.ml" }.
// THe `dependencies` key maps each input file to the set of modules they use, with the structure
// { "file": "/path/to/name.ml", deps: [["Qualified", "Module", "Name"], ["List"]] }.
// This function maps the files from `dependencies` to the files from `local`, noting whether a signature exists for
// each module.
func consDeps(dir string, codept Codept) Deps {
  local := make(map[string]string)
  intfs := make(map[string][]string)
  mods := make(map[string][]string)
  sources := make(Deps)
  for _, loc := range codept.Local {
    src := loc.Ml
    if src == "" { src = loc.Mli }
    local[modulePath(loc.Module)] = depName(src)
  }
  for _, src := range codept.Dependencies {
    if filepath.Dir(src.File) == dir {
      var deps []string
      for _, ds := range src.Deps {
        dep := local[modulePath(ds)]
        if dep != "" { deps = append(deps, dep) }
      }
      name := depName(src.File)
      if filepath.Ext(src.File) == ".mli" { intfs[name] = deps } else { mods[name] = deps }
    }
  }
  for src, deps := range mods { sources[src] = consSource(src, intfs, deps) }
  for src, deps := range intfs {
    if _, mod := mods[src]; !mod { sources[src] = Source{Name: src, Intf: false, Virtual: true, Deps: deps} }
  }
  return sources
}

// While codept is able to scan a directory, there's no way to exclude subdirectories, so files have to be specified
// explicitly.
// In some cases, for example when the module `Stdlib.List` is used, codept will list modules without prefix (e.g.
// `List`). If there is a local module of the same name, this will cause a false positive. Therefore, the input files
// are specified as `Okapi[foo.ml,bar.ml]`, which will make local modules appear as `["Okapi", "List"]` in the output,
// disambiguating them sufficiently.
func runCodept(dir string, files []string) []byte {
  var paths []string
  for _, file := range files {
    if filepath.Ext(file) == ".ml" || filepath.Ext(file) == ".mli" {
      paths = append(paths, dir + "/" + file)
    }
  }
  args := []string{"-native", "-deps", "Okapi[" + strings.Join(paths, ",") + "]"}
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
