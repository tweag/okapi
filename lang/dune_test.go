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
  target1 := DuneLib{
  	Name: "sub_lib",
  	Modules: nil,
  	Flags: []string{"-open", "Angstrom"},
  	Libraries: []DuneLibDep{
      DuneLibOpam{"angstrom"},
      DuneLibOpam{"re"},
      DuneLibOpam{"ipaddr"},
      DuneLibSelect{ModuleChoice{"final.ml", []ModuleAlt{
        {"angstrom", "choice1.ml"},
        {"", "choice2.ml"},
      }}},
    },
  	Auto: true,
  	Wrapped: true,
  	Ppx: false,
  	Preprocess: nil,
  }
  target2 := DuneLib{
  	Name: "sub_extra_lib",
  	Modules: []string{"foo", "bar"},
  	Flags: nil,
  	Libraries: nil,
  	Auto: false,
  	Wrapped: true,
  	Ppx: true,
  	Preprocess: []string{"ppx_inline_test"},
  }
  targets := []DuneLib{target1, target2}
  if !reflect.DeepEqual(output, targets) {
    t.Fatalf("Dune library differs.\nOutput:\n%#v\nTarget:\n%#v", output, targets)
  }
}
