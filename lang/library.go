package okapi

import (
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/bazelbuild/bazel-gazelle/rule"
)

type KeyValue struct {
	key   string
	value string
}

type ModuleAlt struct {
	cond   string
	choice string
}

type ModuleChoice struct {
	out  string
	alts []ModuleAlt
}

func ppxName(libName string) string { return "ppx_" + libName }

func addAttrs(slug string, r *rule.Rule, kind PpxKind) {
	if ppx, isDirect := kind.(PpxDirect); isDirect {
		r.SetAttr("ppx", ":"+ppxName(slug))
		r.SetAttr("ppx_print", "@ppx//print:text")
		if contains("ppx_inline_test", ppx.deps) {
			r.SetAttr("ppx_tags", []string{"inline-test"})
		}
	}
}

func extraRules(kind PpxKind, slug string) []RuleResult {
	if ppx, isDirect := kind.(PpxDirect); isDirect {
		return ppx.exe(slug)
	}
	return nil
}

func libSuffix(library bool) string {
	if library {
		return "library"
	} else {
		return "archive"
	}
}

type LibraryKind interface {
	ruleKind(library bool) string
	ppx() bool
	wrapped() bool
}

type LibNsPpx struct{}
type LibNs struct{}
type LibPpx struct{}
type LibPlain struct{}

func (LibNsPpx) ruleKind(library bool) string { return "ppx_ns_" + libSuffix(library) }
func (LibNs) ruleKind(library bool) string    { return "ocaml_ns_" + libSuffix(library) }
func (LibPpx) ruleKind(library bool) string   { return "ppx_" + libSuffix(library) }
func (LibPlain) ruleKind(library bool) string { return "ocaml_" + libSuffix(library) }

func (LibNsPpx) ppx() bool { return true }
func (LibNs) ppx() bool    { return false }
func (LibPpx) ppx() bool   { return true }
func (LibPlain) ppx() bool { return false }

func (LibNsPpx) wrapped() bool { return true }
func (LibNs) wrapped() bool    { return true }
func (LibPpx) wrapped() bool   { return false }
func (LibPlain) wrapped() bool { return false }

type ExeKind interface {
	ruleKind(test bool) string
	ppx() bool
}

type ExePpx struct{}
type ExePlain struct{}

func (ExePpx) ruleKind(test bool) string {
	if test {
		return "ppx_test"
	} else {
		return "ppx_executable"
	}
}
func (exe ExePlain) ruleKind(test bool) string {
	if test {
		return "ocaml_test"
	} else {
		return "ocaml_executable"
	}
}

func (ExePpx) ppx() bool   { return true }
func (ExePlain) ppx() bool { return false }

type ComponentKind interface {
	componentRule(component Component, library bool) *rule.Rule
	extraDeps() []string
}

type Library struct {
	name           ComponentName
	virtualModules []Source
	implements     string
	kind           LibraryKind
}

type Executable struct {
	kind ExeKind
	test bool
}

// TODO store stuff like auto, exclude in annotations
type Component struct {
	name        ComponentName
	sources     *SourceSet
	annotations []string
}

type ComponentSlice []Component

func (s ComponentSlice) Len() int           { return len(s) }
func (s ComponentSlice) Less(i, j int) bool { return s[i].name.name < s[j].name.name }
func (s ComponentSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s ComponentSlice) Sort()              { sort.Sort(s) }

func sortedComponents(srcs []Component) []Component {
	var sources ComponentSlice = srcs
	sources.Sort()
	return sources
}

type SourceSet struct {
	name     string
	sources  []Source
	spec     ModuleSpec
	depsOpam []string
	ppx      PpxKind
	kind     ComponentKind
	flags    []string
	mains    []string
	// TODO merge with sources before constructing
}

type SourceSetSlice []SourceSet

func (s SourceSetSlice) Len() int           { return len(s) }
func (s SourceSetSlice) Less(i, j int) bool { return s[i].name < s[j].name }
func (s SourceSetSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s SourceSetSlice) Sort()              { sort.Sort(s) }

func sortedSourceSets(srcs []SourceSet) []SourceSet {
	var sources SourceSetSlice = srcs
	sources.Sort()
	return sources
}

type Package struct {
	components []Component
	sources    []SourceSet
}

func moduleAttr(wrapped bool) string {
	if wrapped {
		return "submodules"
	} else {
		return "modules"
	}
}

func nsName(name string) string {
	return "#" + strings.Title(strings.ReplaceAll(name, "-", "_"))
}

func targetNames(deps []string) []string {
	var result []string
	for _, dep := range deps {
		result = append(result, ":"+dep)
	}
	sort.Strings(result)
	return result
}

func libraryNames(srcs []Source) []string {
	var result []string
	for _, src := range srcs {
		if src.generator.libraryModule() {
			result = append(result, src.name)
		}
	}
	sort.Strings(result)
	return result
}

func prefixColon(names []string) []string {
	var result []string
	for _, name := range names {
		result = append(result, ":"+name)
	}
	return result
}

func libraryModules(srcs []Source) []string {
	return prefixColon(libraryNames(srcs))
}

func exeModules(set *SourceSet) []string {
	all := libraryNames(set.sources)
	var result []string
	for _, mod := range all {
		found := false
		for _, main := range set.mains {
			if mod == main {
				found = true
			}
		}
		if !found {
			result = append(result, mod)
		}
	}
	return prefixColon(result)
}

func libraryRule(lib Library, component Component, library bool, name string, publicName string) *rule.Rule {
	r := rule.NewRule(lib.kind.ruleKind(library), name)
	mods := append(component.sources.sources, lib.virtualModules...)
	r.SetAttr(moduleAttr(lib.kind.wrapped()), libraryModules(mods))
	if lib.implements != "" {
		r.AddComment("# okapi:implements " + lib.implements)
		r.AddComment("# okapi:implementation " + publicName)
	}
	return r
}

func (lib Library) componentRule(component Component, library bool) *rule.Rule {
	name := component.name
	libName := "lib-" + name.name
	if lib.kind.wrapped() {
		libName = nsName(name.name)
	}
	return libraryRule(lib, component, library, libName, name.public)
}

func (exe Executable) componentRule(component Component, library bool) *rule.Rule {
	name := component.name
	r := rule.NewRule(exe.kind.ruleKind(exe.test), "exe-"+name.public)
	r.SetAttr("main", name.name)
	r.SetAttr("deps", exeModules(component.sources))
	return r
}

func (lib Library) extraDeps() []string {
	if lib.implements == "" {
		return nil
	} else {
		return []string{lib.implements}
	}
}

func (Executable) extraDeps() []string { return nil }

// A rule to be generated by OBazl
type RuleResult struct {
	rule *rule.Rule
	// Opam and local dependencies of this rule (library or executable)
	deps []string
}

func extendAttr(r *rule.Rule, attr string, vs []string) {
	if len(vs) > 0 {
		r.SetAttr(attr, append(r.AttrStrings(attr), vs...))
	}
}

func appendAttr(r *rule.Rule, attr string, v string) {
	r.SetAttr(attr, append(r.AttrStrings(attr), v))
}

func commonAttrs(set SourceSet, r *rule.Rule, deps []string) RuleResult {
	libDeps := append(append(set.depsOpam, set.ppx.depsOpam()...), set.kind.extraDeps()...)
	extendAttr(r, "opts", set.flags)
	if len(deps) > 0 {
		r.SetAttr("deps", targetNames(deps))
	}
	addAttrs(set.name, r, set.ppx)
	return RuleResult{r, libDeps}
}

func sigTarget(src Source) string { return src.name + "__sig" }

func signatureRule(set SourceSet, src Source, deps []string) RuleResult {
	r := rule.NewRule("ocaml_signature", sigTarget(src))
	r.SetAttr("src", ":"+src.name+".mli")
	return commonAttrs(set, r, deps)
}

func virtualSignatureRule(libName string, src Source) *rule.Rule {
	r := rule.NewRule("ocaml_signature", src.name)
	r.SetAttr("src", ":"+src.name+".mli")
	r.AddComment(fmt.Sprintf("# okapi:virt %s", libName))
	return r
}

func moduleRuleName(set SourceSet) string {
	if set.ppx.isPpx() {
		return "ppx_module"
	} else {
		return "ocaml_module"
	}
}

func moduleRule(set SourceSet, src Source, struct_ string, deps []string) RuleResult {
	r := rule.NewRule(moduleRuleName(set), src.name)
	r.SetAttr("struct", struct_)
	if src.intf {
		r.SetAttr("sig", ":"+sigTarget(src))
	} else if lib, isLib := set.kind.(Library); isLib && lib.implements != "" {
		r.AddComment(fmt.Sprintf("# okapi:implements %s", lib.implements))
	}
	return commonAttrs(set, r, deps)
}

func defaultModuleRule(set SourceSet, src Source, deps []string) RuleResult {
	return moduleRule(set, src, ":"+src.name+".ml", deps)
}

func lexRules(set SourceSet, src Source, deps []string) []RuleResult {
	structName := src.name + "_ml"
	lexRule := rule.NewRule("ocaml_lex", structName)
	lexRule.SetAttr("src", ":"+src.name+".mll")
	modRule := moduleRule(set, src, ":"+structName, deps)
	modRule.rule.SetAttr("opts", []string{"-w", "-39"})
	return []RuleResult{{lexRule, nil}, modRule}
}

func remove(name string, deps []string) []string {
	var result []string
	for _, dep := range deps {
		if dep != name {
			result = append(result, dep)
		}
	}
	return result
}

func librarySourceRules(set SourceSet, lib Library) []RuleResult {
	var rules []RuleResult
	var m SourceSlice = lib.virtualModules
	m.Sort()
	for _, src := range m {
		if src.generator == nil {
			log.Fatalf("no generator for %#v", src)
		}
		cleanDeps := remove(src.name, src.deps)
		rules = append(rules, commonAttrs(set, virtualSignatureRule(lib.name.public, src), cleanDeps))
	}
	return rules
}

// If the source was generated, the module rule will be handled by the generator logic.
// This still uses a potential interface though, since that may be supplied unmanaged.
func sourceRule(set SourceSet, src Source) []RuleResult {
	var rules []RuleResult
	cleanDeps := remove(src.name, src.deps)
	if src.intf {
		rules = append(rules, signatureRule(set, src, cleanDeps))
	}
	if _, isNoGen := src.generator.(NoGenerator); isNoGen {
		rules = append(rules, defaultModuleRule(set, src, cleanDeps))
	} else if _, isLexer := src.generator.(Lexer); isLexer {
		rules = append(rules, lexRules(set, src, cleanDeps)...)
	} else if _, isChoice := src.generator.(Choice); isChoice {
		rules = append(rules, defaultModuleRule(set, src, cleanDeps))
	} else {
		log.Fatalf("no generator for %#v", src)
	}
	return rules
}

func sourceRules(set SourceSet) []RuleResult {
	var rules []RuleResult
	rules = append(rules, extraRules(set.ppx, set.name)...)
	var m SourceSlice = set.sources
	m.Sort()
	for _, src := range m {
		rules = append(rules, sourceRule(set, src)...)
	}
	if lib, isLib := set.kind.(Library); isLib {
		rules = append(rules, librarySourceRules(set, lib)...)
	}
	return rules
}

func component(component Component, library bool) []RuleResult {
	var result []RuleResult
	r := component.sources.kind.componentRule(component, library)
	if component.sources.spec.auto() {
		r.AddComment("# okapi:auto")
	}
	r.AddComment("# okapi:public_name " + component.name.public)
	r.SetAttr("visibility", []string{"//visibility:public"})
	result = append(result, RuleResult{r, component.sources.depsOpam})
	return result
}

type ComponentSources struct {
	component ComponentSpec
	sources   *SourceSet
}

func libChoices(sets map[int]SourceSet) map[string]bool {
	result := make(map[string]bool)
	for _, set := range sets {
		for _, mod := range set.sources {
			if c, isChoice := mod.generator.(Choice); isChoice {
				for _, a := range c.alts {
					result[depName(a.choice)] = true
				}
			}
		}
	}
	return result
}

func autoModules(sets map[int]SourceSet, sources Deps) []Source {
	knownModules := make(map[string]bool)
	choices := libChoices(sets)
	var auto []Source
	for _, set := range sets {
		for _, mod := range set.sources {
			knownModules[mod.name] = true
		}
		if lib, isLib := set.kind.(Library); isLib {
			for _, mod := range lib.virtualModules {
				knownModules[mod.name] = true
			}
		}
	}
	for name, src := range sources {
		if _, exists := knownModules[name]; !exists {
			if _, isChoice := choices[name]; !isChoice {
				auto = append(auto, src)
			}
		}
	}
	return auto
}

// Assign source files that aren't mentioned explicitly in any module set to an auto set.
// This can be either one that specifies no modules (or only :standard) or one that specifies an exclusion pattern (in
// which case the auto modules are filtered).
func filterAuto(auto []Source, spec ModuleSpec) []Source {
	if _, isAuto := spec.(AutoModules); isAuto {
		return auto
	} else if exclude, isExclude := spec.(ExcludeModules); isExclude {
		var result []Source
		for _, src := range auto {
			found := false
			for _, ex := range exclude.modules {
				if ex == src.name {
					found = true
				}
			}
			if !found {
				result = append(result, src)
			}
		}
		return result
	} else {
		return nil
	}
}

func assignAuto(auto []Source, sets map[int]SourceSet) map[int]SourceSet {
	result := make(map[int]SourceSet)
	for i, set := range sets {
		set.sources = append(set.sources, filterAuto(auto, set.spec)...)
		result[i] = set
	}
	return result
}

// Create final source sets from dune module specs, assigning generated modules and choices.
// Then pair components with a pointer to the associated source set.
func componentsWithSources(pkg PackageSpec, generated map[int][]string, deps Deps) ([]ComponentSources, []SourceSet) {
	var components []ComponentSources
	sourceSets := make(map[int]SourceSet)
	for i, mods := range pkg.modules {
		srcs := moduleSources(append(mods.modules.names(), generated[i]...), deps, mods.choices)
		sourceSets[i] = SourceSet{
			name:     fmt.Sprintf("set-%d", i),
			sources:  srcs,
			spec:     mods.modules,
			depsOpam: mods.depsOpam,
			ppx:      mods.ppx,
			kind:     mods.kind.toObazl(mods.ppx, deps),
			flags:    mods.flags,
			mains:    mods.mains,
		}
	}
	auto := autoModules(sourceSets, deps)
	withAuto := assignAuto(auto, sourceSets)
	var sourcesSlice []SourceSet
	for _, ss := range withAuto {
		sourcesSlice = append(sourcesSlice, ss)
	}
	for _, comp := range pkg.components {
		ss := withAuto[comp.modules]
		components = append(components, ComponentSources{comp, &ss})
	}
	return components, sourcesSlice
}

func specComponent(comp ComponentSources) Component {
	return Component{
		name:        comp.component.name,
		sources:     comp.sources,
		annotations: nil,
	}
}

func specComponents(spec PackageSpec, sources Deps) Package {
	generated := assignGenerated(spec)
	withSources, sets := componentsWithSources(spec, generated, sources)
	var result []Component
	for _, comp := range withSources {
		result = append(result, specComponent(comp))
	}
	return Package{result, sets}
}

// Update an existing build that has been manually amended by the user to contain more than one library.
// In that case, all submodule assignments are static, and only the module/signature rules are updated.
// TODO when `select` directives are used from dune, they don't create module rules for the choices.
// When gazelle is then run in update mode, they will be created.
// Either check for rules that select one of the choices or add exclude rules in comments.
func multilib(spec PackageSpec, sources Deps, library bool) []RuleResult {
	pkg := specComponents(spec, sources)
	var rules []RuleResult
	for _, srcSet := range sortedSourceSets(pkg.sources) {
		rules = append(rules, sourceRules(srcSet)...)
	}
	for _, comp := range sortedComponents(pkg.components) {
		rules = append(rules, component(comp, library)...)
	}
	return rules
}
