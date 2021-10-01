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

var defaultLibKind = LibSpec{
  wrapped: true,
  virtualModules: nil,
  implements: "",
}

func TestDuneParse(t *testing.T) {
  sexp := parseDune(duneFile)
  output := decodeDuneConfig("test", sexp)
  target1 := DuneComponent{
    core: DuneComponentCore{
      names: []ComponentName{{
          name: "sub_lib",
          public: "sub-lib",
      }},
      flags: []string{"-open", "Angstrom"},
      auto: true,
    },
    modules: AutoModules{},
    libraries: []DuneLibDep{
      DuneLibOpam{"angstrom"},
      DuneLibOpam{"re"},
      DuneLibOpam{"ipaddr"},
      DuneLibSelect{ModuleChoice{"final.ml", []ModuleAlt{
        {"angstrom", "choice1.ml"},
        {"", "choice2.ml"},
      }}},
    },
    ppx: false,
    preprocess: nil,
    kind: defaultLibKind,
  }
  target2 := DuneComponent{
    core: DuneComponentCore{
      names: []ComponentName{{
        name: "sub_extra_lib",
        public: "sub-extra-lib",
      }},
      flags: nil,
      auto: false,
    },
    modules: ConcreteModules{[]string{"foo", "bar"}},
    libraries: nil,
    ppx: true,
    preprocess: []string{"ppx_inline_test"},
    kind: defaultLibKind,
  }
  targets := []DuneComponent{target1, target2}
  conf := DuneConfig{targets, nil}
  if !reflect.DeepEqual(output, conf) {
    t.Fatalf("Dune library differs.\nOutput:\n%#v\nTarget:\n%#v", output, targets)
  }
}

func TestDuneAssignGenerated(t *testing.T) {
  comp1 := DuneComponent{
    core: DuneComponentCore{names: []ComponentName{{name: "comp1", public: ""}}, auto: false},
    modules: ConcreteModules{[]string{"lex1", "mod1"}},
  }
  comp2 := DuneComponent{
    core: DuneComponentCore{names: []ComponentName{{name: "comp2", public: ""}}, auto: true},
    modules: AutoModules{},
  }
  comp3 := DuneComponent{
    core: DuneComponentCore{names: []ComponentName{{name: "comp3", public: ""}}, auto: false},
    modules: ConcreteModules{[]string{"lex2", "mod2"}},
  }
  comps := []DuneComponent{comp1, comp2, comp3}
  generated := []string{"lex1", "lex2", "lex3"}
  conf := DuneConfig{comps, generated}
  spec := duneToSpec(conf)
  result := assignGenerated(spec)
  target := map[string][]string {
    "comp1": {"lex1"},
    "comp2": {"lex3"},
    "comp3": {"lex2"},
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
	deps["Module1"] = Source {name: "foo", intf: false, virtual: false, deps: []string{}, generator: NoGenerator{}}
	deps["Module2"] = Source {name: "bar", intf: false, virtual: false, deps: []string{}, generator: NoGenerator{}}
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
