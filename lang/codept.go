package okapi

import (
  "encoding/json"
  "log"
  "os"
  "os/exec"
  "path/filepath"
  "sort"
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

type Generator interface {
  remove() bool
  libraryModule() bool
}
type NoGenerator struct {}
type Lexer struct {}
type Choice struct {
  alts []ModuleAlt
}

func (NoGenerator) remove() bool { return false }
func (Lexer) remove() bool { return true }
func (Choice) remove() bool { return false }

func (NoGenerator) libraryModule() bool { return true }
func (Lexer) libraryModule() bool { return false }
func (Choice) libraryModule() bool { return true }

type CodeptSource struct {
  name string
  ext string
  path string
  codeptPath string
  generator Generator
}

type Source struct {
  name string
  intf bool
  virtual bool
  deps []string
  generator Generator
}

func sortSources(srcs []Source) {
  sort.Slice(srcs, func(i, j int) bool { return srcs[i].name < srcs[j].name })
}

type Deps = map[string]Source

func depName(file string) string {
  return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
}

// TODO remove intf from deps?
func consSource(name string, intfs map[string][]string, deps []string, codept CodeptSource) Source {
  intf, hasIntf := intfs[name]
  return Source{
    name: name,
    intf: hasIntf,
    virtual: false,
    deps: append(deps, intf...),
    generator: codept.generator,
  }
}

func modulePath(segments []string) string { return strings.Join(segments, ".") }

func runLexer(dir string, file string) string {
  ml := file[:len(file) - 1]
  mlpath := filepath.Join(dir, ml)
  path := filepath.Join(dir, file)
  if _, err := os.Stat(mlpath); err == nil {
    log.Fatalf("ocamllex module for %s already exists.", path)
  }
  cmd := exec.Command("ocamllex", path)
  out, err := cmd.CombinedOutput()
  if err != nil {
    log.Fatalf("ocamllex failed for %s with %#v: %s\n", path, err.Error(), string(out))
  }
  return mlpath
}

func prepareSources(dir string, files []string) map[string]CodeptSource {
  result := make(map[string]CodeptSource)
  for _, file := range files {
    path := filepath.Join(dir, file)
    ext := filepath.Ext(file)
    name := depName(file)
    if ext == ".ml" || ext == ".mli" {
      result[file] = CodeptSource{
        name: name,
        ext: ext,
        path: path,
        codeptPath: path,
        generator: NoGenerator{},
      }
    } else if ext == ".mll" {
      ml := runLexer(dir, file)
      result[name + ".ml"] = CodeptSource{
        name: name,
        ext: ext,
        path: path,
        codeptPath: ml,
        generator: Lexer{},
      }
    }
  }
  return result
}

// The `local` key in the codept output maps all used modules to their defining source files with the structure
// { "module": ["Qualified", "Module", "Name"], "ml": "/path/to/name.ml" }.
// THe `dependencies` key maps each input file to the set of modules they use, with the structure
// { "file": "/path/to/name.ml", deps: [["Qualified", "Module", "Name"], ["List"]] }.
// This function maps the files from `dependencies` to the files from `local`, noting whether a signature exists for
// each module.
func consDeps(dir string, codept Codept, codeptSources map[string]CodeptSource) Deps {
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
  for src, deps := range mods { sources[src] = consSource(src, intfs, deps, codeptSources[src + ".ml"]) }
  for src, deps := range intfs {
    if _, mod := mods[src]; !mod {
      sources[src] = Source{
        name: src,
        intf: false,
        virtual: true,
        deps: deps,
        generator: NoGenerator{},
      }
    }
  }
  return sources
}

// While codept is able to scan a directory, there's no way to exclude subdirectories, so files have to be specified
// explicitly.
// In some cases, for example when the module `Stdlib.List` is used, codept will list modules without prefix (e.g.
// `List`). If there is a local module of the same name, this will cause a false positive. Therefore, the input files
// are specified as `Okapi[foo.ml,bar.ml]`, which will make local modules appear as `["Okapi", "List"]` in the output,
// disambiguating them sufficiently.
func runCodept(dir string, sources map[string]CodeptSource) []byte {
  var paths []string
  for _, src := range sources { paths = append(paths, src.codeptPath) }
  args := []string{"-native", "-deps", "-k", "Okapi[" + strings.Join(paths, ",") + "]"}
  cmd := exec.Command("codept", args...)
  out, err := cmd.Output()
  if err != nil {
    cmdline := "codept " + strings.Join(args, " ")
    log.Fatalf("codept failed for %s with %#v: %s\ncmdline: %s", dir, err.Error(), string(out[:]), cmdline)
  }
  for _, src := range sources {
    if src.generator.remove() { os.Remove(src.codeptPath) }
  }
  return out
}

func Dependencies(dir string, files []string) Deps {
  sources := prepareSources(dir, files)
  out := runCodept(dir, sources)
  var codept Codept
  err := json.Unmarshal(out, &codept)
  if err != nil { log.Fatal("Parsing codept output for " + dir + ":\n" + err.Error() + "\n" + string(out[:])) }
  return consDeps(dir, codept, sources)
}
