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

func TestDune(t *testing.T) {
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
  if !reflect.DeepEqual(output, targets) {
    t.Fatalf("Dune library differs.\nOutput:\n%#v\nTarget:\n%#v", output, targets)
  }
}
