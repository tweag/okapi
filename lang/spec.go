package okapi

// A `modules` stanza (may be absent, in that case auto = true)
type ModuleSpec interface {
  names() []string
  specifies(mod string) bool
  auto() bool
}

// AutoModules implements ModuleSpec
// Either `:standard` or unspecified
// Note: limitation: a `modules` stanza can contain both `:standard` and concrete modules
type AutoModules struct {}

// ConcreteModules implements ModuleSpec
type ConcreteModules struct {
  modules []string
}

// ExcludeModules implements ModuleSpec
type ExcludeModules struct {
  modules []string
}

func (AutoModules) names() []string { return nil }
func (spec ConcreteModules) names() []string { return spec.modules }
func (ExcludeModules) names() []string { return nil }

func (AutoModules) specifies(string) bool { return false }
func (spec ConcreteModules) specifies(target string) bool { return contains(target, spec.modules) }
func (ExcludeModules) specifies(string) bool { return false }

func (AutoModules) auto() bool { return true }
func (ConcreteModules) auto() bool { return false }
func (ExcludeModules) auto() bool { return true }

type ComponentName struct {
  name string
  public string
}

type ComponentCore struct {
  name ComponentName
  flags []string
  auto bool
}

type KindSpec interface {
  toObazl(PpxKind, Deps) ComponentKind
}

// LibSpec implements KindSpec
type LibSpec struct {
  wrapped bool
  virtualModules []string
  implements string
}

// ExeSpec implements KindSpec
type ExeSpec struct {}

// LibSpec implements KindSpec
func (lib LibSpec) toObazl(ppx PpxKind, sources Deps) ComponentKind {
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

// ExeSpec implements KindSpec
func (ExeSpec) toObazl(ppx PpxKind, sources Deps) ComponentKind {
  var kind ExeKind = ExePlain{}
  if ppx.isPpx() { kind = ExePpx{} }
  return Executable{
    kind: kind,
  }
}

// Executable | Library | Test
type ComponentSpec struct {
  core ComponentCore
  modules ModuleSpec // Could only be module names ([]string) and let PackageSpec store the module data
  depsOpam []string
  ppx PpxKind
  choices []Source
  kind KindSpec
}

// Directory (or Dune file or Bazel build file)
type PackageSpec struct {
  components []ComponentSpec
  modules []ModuleSpec
  generated []string
}
