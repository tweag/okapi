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
