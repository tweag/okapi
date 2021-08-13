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
  output := DecodeDuneConfig("test", sexp)
  target1 := DuneComponent{
    name: "sub_lib",
    publicName: "sub-lib",
    modules: nil,
    flags: []string{"-open", "Angstrom"},
    libraries: []DuneLibDep{
      DuneLibOpam{"angstrom"},
      DuneLibOpam{"re"},
      DuneLibOpam{"ipaddr"},
      DuneLibSelect{ModuleChoice{"final.ml", []ModuleAlt{
        {"angstrom", "choice1.ml"},
        {"", "choice2.ml"},
      }}},
    },
    auto: true,
    ppx: false,
    preprocess: nil,
    kind: DuneLib{
      wrapped: true,
      virtualModules: nil,
      implements: "",
    },
  }
  target2 := DuneComponent{
    name: "sub_extra_lib",
    publicName: "sub-extra-lib",
    modules: []string{"foo", "bar"},
    flags: nil,
    libraries: nil,
    auto: false,
    ppx: true,
    preprocess: []string{"ppx_inline_test"},
    kind: DuneLib{
      wrapped: true,
      virtualModules: nil,
      implements: "",
    },
  }
  targets := []DuneComponent{target1, target2}
  conf := DuneConfig{targets, nil}
  if !reflect.DeepEqual(output, conf) {
    t.Fatalf("Dune library differs.\nOutput:\n%#v\nTarget:\n%#v", output, targets)
  }
}

func TestDuneAssignGenerated(t *testing.T) {
  comp1 := DuneComponent{name: "comp1", auto: false, modules: []string{"lex1", "mod1"}}
  comp2 := DuneComponent{name: "comp2", auto: true}
  comp3 := DuneComponent{name: "comp3", auto: false, modules: []string{"lex2", "mod2"}}
  comps := []DuneComponent{comp1, comp2, comp3}
  generated := []string{"lex1", "lex2", "lex3"}
  conf := DuneConfig{comps, generated}
  result := assignDuneGenerated(conf)
  target := map[string][]string {
    "comp1": {"lex1"},
    "comp2": {"lex3"},
    "comp3": {"lex2"},
  }
  if !reflect.DeepEqual(result, target) {
    t.Fatalf("Generators weren't assigned correctly:\n\n%#v\n\n%#v", result, target)
  }
}
