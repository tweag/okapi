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
  output := DecodeDuneConfig("test", sexp)
  target1 := DuneComponent{
    core: ComponentCore{
      name: "sub_lib",
      publicName: "sub-lib",
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
    core: ComponentCore{
      name: "sub_extra_lib",
      publicName: "sub-extra-lib",
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
    core: ComponentCore{name: "comp1", auto: false},
    modules: ConcreteModules{[]string{"lex1", "mod1"}},
  }
  comp2 := DuneComponent{
    core: ComponentCore{name: "comp2", auto: true},
    modules: AutoModules{},
  }
  comp3 := DuneComponent{
    core: ComponentCore{name: "comp3", auto: false},
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
