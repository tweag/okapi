package bazel_test

import (
  "io/ioutil"
  "log"
  "path/filepath"
  "strings"
  "testing"

  "github.com/bazelbuild/rules_go/go/tools/bazel_testing"
)

var testArgs = bazel_testing.Args{
  Main: `
-- BUILD.bazel --
load("@okapi//bzl:generate.bzl", "generate")

generate()
-- a/a2.ml --
open F1

module A2 = struct
  let a1 a = F1.m1 a
end
-- a/a2.mli --
module A2 : sig
  val a1 : int -> int
end
-- a/a3.ml --
open F1
open A2

module A3 = struct
  let a3 a = A2.a1 (F1.m1 a)
end
-- a/f1.ml --
module F1 = struct
  let m1 a = a
end
-- a/f1.mli --
module F1 : sig
  val m1 : int -> int
end
-- a/sub/dune --
(library
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
 (modules foo bar))
-- a/sub/bar.ml --
module Bar = struct
  let sub a = a
end
-- a/sub/choice1.ml --
-- a/sub/choice2.ml --
-- a/sub/foo.ml --
module Foo = struct
  let sub a = a
end
-- a/sub/sub.ml --
module Sub = struct
  let sub a = a
end
`,
  WorkspaceSuffix: `
load("@okapi//bzl:deps.bzl", "okapi_deps")
okapi_deps()

load("@okapi//bzl:setup.bzl", "okapi_setup_legacy")
okapi_setup_legacy()
`,
}

const aBuildTarget = `load("@obazl_rules_ocaml//ocaml:rules.bzl", "ocaml_module", "ocaml_ns_library", "ocaml_signature")

ocaml_module(
    name = "a2",
    sig = ":a2_sig",
    struct = ":a2.ml",
    deps = [":f1"],
)

ocaml_signature(
    name = "a2_sig",
    src = ":a2.mli",
    deps = [":f1"],
)

ocaml_module(
    name = "a3",
    struct = ":a3.ml",
    deps = [
        ":a2",
        ":f1",
    ],
)

ocaml_module(
    name = "f1",
    sig = ":f1_sig",
    struct = ":f1.ml",
)

ocaml_signature(
    name = "f1_sig",
    src = ":f1.mli",
)

ocaml_ns_library(
    name = "#A",
    submodules = [
        ":a2",
        ":a3",
        ":f1",
    ],
    visibility = ["//visibility:public"],
)
`

func TestBuild(t *testing.T) {
  if err := bazel_testing.RunBazel("run", "//:gazelle"); err != nil { t.Fatal(err) }
  output, err := bazel_testing.BazelOutput("info", "workspace")
  ws := string(output[:])
  if err != nil { t.Fatal(err) }
  log.Print(ws)
  aBuildFile := filepath.Join(strings.TrimSpace(ws), "a", "BUILD.bazel")
  aBuildBytes, err1 := ioutil.ReadFile(aBuildFile)
  if err1 != nil { t.Fatal(err1) }
  aBuild := string(aBuildBytes)
  if aBuild != aBuildTarget { t.Fatal("a/BUILD.bazel doesn't match:\n" + aBuild) }
}

func TestMain(m *testing.M) { bazel_testing.TestMain(m, testArgs) }
