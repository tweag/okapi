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
}

type Deps = map[Source][]string

func depName(file string) string {
  return strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
}

func consSource(name string, sigs map[string][]string) Source {
  _, intf := sigs[name]
  return Source{name, intf}
}

func consDeps(codept Codept) Deps {
  local := make(map[string]string)
  sigs := make(map[string][]string)
  mods := make(map[string][]string)
  sources := make(Deps)
  for _, loc := range codept.Local {
    for _, mod := range loc.Module { local[mod] = depName(loc.Ml) }
  }
  for _, src := range codept.Dependencies {
    var deps []string
    for _, ds := range src.Deps {
      for _, d := range ds { deps = append(deps, local[d]) }
    }
    name := depName(src.File)
    if filepath.Ext(src.File) == ".mli" { sigs[name] = deps } else { mods[name] = deps }
  }
  for src, deps := range mods { sources[consSource(src, sigs)] = deps }
  return sources
}

func Dependencies(dir string) Deps {
  cmd := exec.Command("codept", "-native", "-deps", dir)
  out, err := cmd.CombinedOutput()
  if err != nil { log.Fatal("codept failed: " + string(out[:])) }
  var codept Codept
  json.Unmarshal(out, &codept)
  return consDeps(codept)
}
