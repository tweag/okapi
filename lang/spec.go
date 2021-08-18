package okapi

type ModuleSpec interface {
  names() []string
  specifies(mod string) bool
  auto() bool
}
type AutoModules struct {}
type ConcreteModules struct {
  modules []string
}
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

type ComponentCore struct {
  name string
  publicName string
  flags []string
  auto bool
}

type KindSpec interface {
  toObazl(PpxKind, Deps) ComponentKind
}

type LibSpec struct {
  wrapped bool
  virtualModules []string
  implements string
}

type ExeSpec struct {}

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

func (ExeSpec) toObazl(ppx PpxKind, sources Deps) ComponentKind {
  var kind ExeKind = ExePlain{}
  if ppx.isPpx() { kind = ExePpx{} }
  return Executable{
    kind: kind,
  }
}

type ComponentSpec struct {
  core ComponentCore
  modules ModuleSpec
  depsOpam []string
  ppx PpxKind
  choices []Source
  kind KindSpec
}

type PackageSpec struct {
  components []ComponentSpec
  generated []string
}
