package okapi

import (
	"reflect"
	"testing"
)

func checkOutput(t *testing.T, output interface{}, target interface{}) {
	if !reflect.DeepEqual(output, target) {
		t.Fatalf("Sexp parser produced differing result.\nOutput:\n%#v\nTarget:\n%#v", output, target)
	}
}

func test(t *testing.T, input string, target []SexpNode) { checkOutput(t, parseSexp(input), target) }

func test1(t *testing.T, input string, target SexpNode) { test(t, input, []SexpNode{target}) }

func TestSexp(t *testing.T) {
	test1(
		t,
		"(flags (:standard -open Lib))",
		consSexpList(
			SexpString{"flags"},
			consSexpList(
				SexpString{":standard"},
				SexpString{"-open"},
				SexpString{"Lib"},
			),
		),
	)
	test1(
		t,
		"(libraries dep (select final.ml from (dep -> choice1.ml) (-> choice2.ml)))",
		consSexpList(
			SexpString{"libraries"},
			SexpString{"dep"},
			consSexpList(
				SexpString{"select"},
				SexpString{"final.ml"},
				SexpString{"from"},
				consSexpList(
					SexpString{"dep"},
					SexpString{"->"},
					SexpString{"choice1.ml"},
				),
				consSexpList(
					SexpString{"->"},
					SexpString{"choice2.ml"},
				),
			),
		),
	)
}
