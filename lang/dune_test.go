package okapi

import (
	"reflect"
	"testing"
)

const duneFile = `(library
 (name sub_lib)
 (public_name sub-lib)
 (flags (:standard -open Angstrom))
 (libraries
   angstrom
   re
   ipaddr
   (select final.ml from
     (angstrom -> choice1.ml)
     (-> choice2.ml))
  ))

(library
 (name sub_extra_lib)
 (public_name sub-extra-lib)
 (preprocess (pps ppx_inline_test))
 (modules foo bar))
`

func TestDuneParse(t *testing.T) {
	sexp := parseDune(duneFile)
	output := decodeDuneConfig("test", sexp)
	target1 := DuneComponent{
		core: DuneComponentCore{
			names: []ComponentName{{
				name:   "sub_lib",
				public: "sub-lib",
			}},
			flags: []string{"-open", "Angstrom"},
		},
		modulesIndex: 0,
		libraries: []DuneLibDep{
			DuneLibOpam{"angstrom"},
			DuneLibOpam{"re"},
			DuneLibOpam{"ipaddr"},
			DuneLibSelect{ModuleChoice{"final.ml", []ModuleAlt{
				{"angstrom", "choice1.ml"},
				{"", "choice2.ml"},
			}}},
		},
		ppx:        false,
		preprocess: nil,
		kind: LibSpec{
			name:           ComponentName{"sub_lib", "sub-lib"},
			wrapped:        true,
			virtualModules: nil,
			implements:     "",
		},
	}
	target2 := DuneComponent{
		core: DuneComponentCore{
			names: []ComponentName{{
				name:   "sub_extra_lib",
				public: "sub-extra-lib",
			}},
			flags: nil,
		},
		modulesIndex: 1,
		libraries:    nil,
		ppx:          true,
		preprocess:   []string{"ppx_inline_test"},
		kind: LibSpec{
			name:           ComponentName{"sub_extra_lib", "sub-extra-lib"},
			wrapped:        true,
			virtualModules: nil,
			implements:     "",
		},
	}
	conc := ConcreteModules{[]string{"foo", "bar"}}
	targets := []DuneComponent{target1, target2}
	mods := map[int]ModuleSpec{0: AutoModules{}, 1: conc}
	conf := DuneConfig{targets, nil, mods}
	if !reflect.DeepEqual(output, conf) {
		t.Fatalf("Dune library differs.\nOutput:\n%#v\nTarget:\n%#v", output, conf)
	}
}

func TestDuneAssignGenerated(t *testing.T) {
	mods := map[int]ModuleSpec{
		0: ConcreteModules{[]string{"lex1", "mod1"}},
		1: AutoModules{},
		2: ConcreteModules{[]string{"lex2", "mod2"}},
	}
	comp1 := DuneComponent{
		core:         DuneComponentCore{names: []ComponentName{{name: "comp1", public: ""}}},
		modulesIndex: 0,
	}
	comp2 := DuneComponent{
		core:         DuneComponentCore{names: []ComponentName{{name: "comp2", public: ""}}},
		modulesIndex: 1,
	}
	comp3 := DuneComponent{
		core:         DuneComponentCore{names: []ComponentName{{name: "comp3", public: ""}}},
		modulesIndex: 2,
	}
	comps := []DuneComponent{comp1, comp2, comp3}
	generated := []string{"lex1", "lex2", "lex3"}
	conf := DuneConfig{comps, generated, mods}
	spec := duneToSpec(conf)
	result := assignGenerated(spec)
	target := map[int][]string{
		0: {"lex1"},
		1: {"lex3"},
		2: {"lex2"},
	}
	if !reflect.DeepEqual(result, target) {
		t.Fatalf("Generators weren't assigned correctly:\n\n%#v\n\n%#v", result, target)
	}
}

func TestDuneExecutables(t *testing.T) {
	const duneFile = `
    (executables
      (names Executable1 Executable2)
      (modules Module1 Module2)
      (libraries library1 library2)
    )
    `
	sexp := parseDune(duneFile)
	duneConfig := decodeDuneConfig("test", sexp)
	spec := duneToSpec(duneConfig)
	deps := make(map[string]Source)
	deps["Module1"] = Source{name: "foo", intf: false, virtual: false, deps: []string{}, generator: NoGenerator{}}
	deps["Module2"] = Source{name: "bar", intf: false, virtual: false, deps: []string{}, generator: NoGenerator{}}
	results := multilib(spec, deps, false)
	if len(results) != 4 {
		t.Logf("Incorrect number of rules generated!")
		t.Logf("Expected 4 rules (2 x 1 per module + 2 x 1 per executable).")
		t.Logf("Instead got %v", len(results))
		for _, result := range results {
			t.Logf("%v (kind: %v)", result.rule.Name(), result.rule.Kind())
		}
		t.FailNow()
	}
}
