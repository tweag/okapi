package okapi

import (
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"
	"strings"
)

type DuneLibDep interface{}
type DuneLibOpam struct{ name string }
type DuneLibSelect struct{ Choice ModuleChoice }

type DuneComponentCore struct {
	names []ComponentName
	flags []string
}

// Either Executable Library
type DuneComponent struct {
	core       DuneComponentCore
	modules    int
	libraries  []DuneLibDep
	ppx        bool
	preprocess []string
	kind       KindSpec
}

type DuneConfig struct {
	components []DuneComponent
	generated  []string
	modules    map[int]ModuleSpec
	// TODO add here the map[moduleKey]ModuleSpec here
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

type SexpComponent struct {
	name string
	data SexpMap
}

func (lib SexpComponent) fatalf(msg string, v ...interface{}) {
	log.Fatalf(fmt.Sprintf("dune library %s: ", lib.name)+msg, v...)
}

func (lib SexpComponent) list(attr string) []string {
	var result []string
	raw := lib.data.Values[attr]
	if raw != nil {
		items, err := sexpStrings(raw)
		if err != nil {
			lib.fatalf("attr %s is not a list of strings: %s: %#v", attr, err, raw)
		}
		for _, item := range items {
			if item[:1] != ":" {
				result = append(result, item)
			}
		}
	}
	return result
}

func (lib SexpComponent) stringOr(key string, def string) string {
	raw, exists := lib.data.Values[key]
	if exists {
		value, stringErr := raw.String()
		if stringErr != nil {
			lib.fatalf("%s isn't a string: %#v", key, raw)
		}
		return value
	} else {
		return def
	}
}

func (lib SexpComponent) stringOptional(key string) string { return lib.stringOr(key, "") }

func (lib SexpComponent) string(key string) string {
	value := lib.stringOptional(key)
	if value == "" {
		lib.fatalf("no `%s` attribute", key)
	}
	return value
}

func duneLibraryDeps(lib SexpComponent) []DuneLibDep {
	var deps []DuneLibDep
	raw := lib.data.Values["libraries"]
	selectString := SexpString{"select"}
	if raw != nil {
		entries, err := raw.List()
		if err != nil {
			lib.fatalf("invalid libraries field: %#v; %s", raw, err)
		}
		for _, entry := range entries {
			s, err := entry.String()
			if err == nil {
				deps = append(deps, DuneLibOpam{s})
			} else {
				sel, err := entry.List()
				if err != nil {
					lib.fatalf("unparsable libraries entry: %#v; %s", sel, err)
				}
				if len(sel) > 3 && sel[0] == selectString {
					var alts []ModuleAlt
					for _, alt := range sel[3:] {
						ss, err := sexpStrings(alt)
						if err == nil && len(ss) == 2 && ss[0] == "->" {
							alts = append(alts, ModuleAlt{"", ss[1]})
						} else if err == nil && len(ss) == 3 && ss[1] == "->" {
							alts = append(alts, ModuleAlt{ss[0], ss[2]})
						} else {
							lib.fatalf("unparsable select alternative: %#v; %s", alt, err)
						}
					}
					final, err := sel[1].String()
					if err != nil {
						lib.fatalf("library %s: invalid type for select file name: %#v; %s", sel, err)
					}
					deps = append(deps, DuneLibSelect{ModuleChoice{final, alts}})
				}
			}
		}
	}
	return deps
}

func dunePreprocessors(lib SexpComponent) []string {
	var result []string
	raw := lib.data.Values["preprocess"]
	if raw != nil {
		if items, err := raw.List(); err == nil {
			for _, item := range items {
				elems, err := item.List()
				pps := SexpString{"pps"}
				if err == nil && len(elems) == 2 && elems[0] == pps {
					pp, stringErr := elems[1].String()
					if stringErr != nil {
						lib.fatalf("dune library %s: pps is not a string: %#v", elems[1])
					}
					result = append(result, pp)
				}
			}
		} else {
			lib.fatalf("invalid `preprocess` directive: %#v", raw)
		}
	}
	return result
}

func duneModules(names []string) ModuleSpec {
	if len(names) == 0 {
		return AutoModules{}
	} else if names[0] == "\\" {
		return ExcludeModules{names[1:]}
	} else {
		return ConcreteModules{names}
	}
}

func duneLibraryKind(lib SexpComponent, name ComponentName) KindSpec {
	wrapped := lib.data.Values["wrapped"] != SexpString{"false"}
	return LibSpec{
		name:           name,
		wrapped:        wrapped,
		virtualModules: lib.list("virtual_modules"),
		implements:     lib.stringOptional("implements"),
	}
}

func duneExeKind(lib SexpComponent) KindSpec {
	if lib.data.Name == "executable" || lib.data.Name == "executables" {
		return ExeSpec{test: false}
	} else if lib.data.Name == "test" || lib.data.Name == "tests" {
		return ExeSpec{test: true}
	}
	return nil
}

func duneComponent(data SexpComponent, names []ComponentName, conf SexpMap, moduleIndex int, kind KindSpec) DuneComponent {
	preproc := dunePreprocessors(data)
	return DuneComponent{
		core: DuneComponentCore{
			names: names,
			flags: data.list("flags"),
		},
		modules:    moduleIndex,
		libraries:  duneLibraryDeps(data),
		ppx:        len(preproc) > 0,
		preprocess: preproc,
		kind:       kind,
	}
}

func duneExecutables(libName string, conf SexpMap, moduleIndex int) DuneComponent {
	data := SexpComponent{libName, conf}
	names := data.list("names")
	public := names
	if _, hasPublic := conf.Values["public_names"]; hasPublic {
		public = data.list("public_names")
	}
	var cnames []ComponentName
	for i, name := range names {
		cnames = append(cnames, ComponentName{name, public[i]})
	}
	return duneComponent(data, cnames, conf, moduleIndex, duneExeKind(data))
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

func decodeDuneConfig(libName string, conf SexpList) DuneConfig {
	var components []DuneComponent
	generated := generatedSources(conf)
	moduleIndex := 0
	modules := make(map[int]ModuleSpec)
	for _, node := range conf.Sub {
		dune, isMap := node.(SexpMap)
		if isMap {
			data := SexpComponent{libName, dune}
			modules[moduleIndex] = duneModules(data.list("modules"))
			if dune.Name == "library" {
				name := data.string("name")
				cname := ComponentName{name, data.stringOr("public_name", name)}
				components = append(components, duneComponent(data, []ComponentName{cname}, dune, moduleIndex, duneLibraryKind(data, cname)))
			} else if dune.Name == "executable" || dune.Name == "test" {
				name := data.string("name")
				cname := ComponentName{name, data.stringOr("public_name", name)}
				components = append(components, duneComponent(data, []ComponentName{cname}, dune, moduleIndex, duneExeKind(data)))
			} else if dune.Name == "executables" || dune.Name == "tests" {
				components = append(components, duneExecutables(libName, dune, moduleIndex))
			}
			moduleIndex += 1
		}
	}
	return DuneConfig{components, generated, modules}
}

func contains(target string, items []string) bool {
	for _, item := range items {
		if target == item {
			return true
		}
	}
	return false
}

func modulesWithSelectOutputs(spec ModuleSpec, libs []DuneLibDep) ModuleSpec {
	if concrete, isConcrete := spec.(ConcreteModules); isConcrete {
		var result []string
		var alts []string
		for _, lib := range libs {
			if sel, isSel := lib.(DuneLibSelect); isSel {
				result = append(result, depName(sel.Choice.out))
				for _, alt := range sel.Choice.alts {
					alts = append(alts, depName(alt.choice))
				}
			}
		}
		for _, mod := range concrete.modules {
			if !contains(mod, alts) {
				result = append(result, mod)
			}
		}
		return ConcreteModules{result}
	} else {
		return spec
	}
}

func duneChoices(libs []DuneLibDep) []Source {
	var choices []Source
	for _, dep := range libs {
		if sel, isSel := dep.(DuneLibSelect); isSel {
			name := depName(sel.Choice.out)
			src := Source{
				name:      name,
				intf:      false,
				virtual:   false,
				deps:      nil,
				generator: Choice{sel.Choice.alts},
			}
			choices = append(choices, src)
		}
	}
	return choices
}

func opamDeps(deps []DuneLibDep) []string {
	var result []string
	for _, dep := range deps {
		ld, isOpam := dep.(DuneLibOpam)
		if isOpam {
			result = append(result, ld.name)
		}
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
		if wrapped {
			return LibNsPpx{}
		} else {
			return LibPpx{}
		}
	} else {
		if wrapped {
			return LibNs{}
		} else {
			return LibPlain{}
		}
	}
}

// Assign modules generated by ocamllex etc. to the appropriate component.
// If the module is listed explicitly in a (modules) stanza, use that one.
// Otherwise, use the auto library/executable.
func assignGenerated(spec PackageSpec) map[int][]string {
	byGen := make(map[string]int)
	result := make(map[int][]string)
	var gens []string
	for _, gen := range spec.generated {
		gens = append(gens, gen)
	}
	for _, gen := range gens {
		for i, srcSpec := range spec.modules {
			_, exists := byGen[gen]
			if srcSpec.modules.auto() {
				if !exists {
					byGen[gen] = i
				}
			} else {
				if srcSpec.modules.specifies(gen) {
					byGen[gen] = i
				}
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
		if c.name == name {
			return true
		}
	}
	return false
}

func untitlecase(name string) string {
	return strings.ToLower(name[:1]) + name[1:]
}

func moduleSources(names []string, sources Deps, choices []Source) []Source {
	var result SourceSlice
	for _, name := range names {
		if src, exists := sources[name]; exists {
			result = append(result, src)
		} else if src, exists := sources[untitlecase(name)]; exists {
			result = append(result, src)
		} else if !isChoice(name, choices) {
			log.Fatalf("Library refers to unknown source `%s`.", name)
		}
	}
	final := append(result, choices...)
	final.Sort()
	return final
}

func duneComponentToSpec(dune DuneComponent, modules ModuleSpec) ([]ComponentSpec, SourcesSpec) {
	ppx := dunePpx(dune.preprocess)
	choices := duneChoices(dune.libraries)
	fullModules := modulesWithSelectOutputs(modules, dune.libraries)
	_, isExe := dune.kind.(ExeSpec)
	var result []ComponentSpec
	var mains []string
	for _, name := range dune.core.names {
		result = append(result, ComponentSpec{
			name:    name,
			modules: dune.modules,
		})
		if isExe {
			mains = append(mains, name.name)
		}
	}
	return result, SourcesSpec{
		modules:  fullModules,
		choices:  choices,
		ppx:      ppx,
		depsOpam: opamDeps(dune.libraries),
		kind:     dune.kind,
		flags:    dune.core.flags,
		mains:    mains,
	}
}

func duneToSpec(config DuneConfig) PackageSpec {
	var components []ComponentSpec
	modules := make(map[int]SourcesSpec)
	for _, comp := range config.components {
		compModules := config.modules[comp.modules]
		comps, fullModules := duneComponentToSpec(comp, compModules)
		components = append(components, comps...)
		modules[comp.modules] = fullModules
	}
	return PackageSpec{
		components: components,
		generated:  config.generated,
		modules:    modules,
	}
}

func findDune(dir string, files []string) string {
	for _, f := range files {
		if f == "dune" {
			return filepath.Join(dir, f)
		}
	}
	return ""
}
