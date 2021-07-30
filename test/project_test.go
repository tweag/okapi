package bazel_test

import (
  "io/ioutil"
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
 (preprocess (pps ppx_inline_test))
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

ocaml_signature(
    name = "a2_sig",
    src = ":a2.mli",
    deps_opam = [],
    opts = [],
    deps = [":f1"],
)

ocaml_module(
    name = "a2",
    deps_opam = [],
    opts = [],
    sig = ":a2_sig",
    struct = ":a2.ml",
    deps = [":f1"],
)

ocaml_module(
    name = "a3",
    deps_opam = [],
    opts = [],
    struct = ":a3.ml",
    deps = [
        ":a2",
        ":f1",
    ],
)

ocaml_signature(
    name = "f1_sig",
    src = ":f1.mli",
    deps_opam = [],
    opts = [],
)

ocaml_module(
    name = "f1",
    deps_opam = [],
    opts = [],
    sig = ":f1_sig",
    struct = ":f1.ml",
)

# okapi:auto
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

const subBuildTarget = `load("@obazl_rules_ocaml//ocaml:rules.bzl", "ocaml_module", "ocaml_ns_library", "ppx_executable", "ppx_module", "ppx_ns_library")

ppx_executable(
    name = "ppx_sub_lib",
    deps_opam = ["ppx_inline_test"],
    main = "@obazl_rules_ocaml//dsl:ppx_driver",
)

ppx_module(
    name = "final",
    deps_opam = [
        "angstrom",
        "re",
        "ipaddr",
    ],
    opts = [
        "-open",
        "Angstrom",
    ],
    ppx = ":ppx_sub_lib",
    ppx_print = "@ppx//print:text",
    struct = ":final.ml",
)

ppx_module(
    name = "sub",
    deps_opam = [
        "angstrom",
        "re",
        "ipaddr",
    ],
    opts = [
        "-open",
        "Angstrom",
    ],
    ppx = ":ppx_sub_lib",
    ppx_print = "@ppx//print:text",
    struct = ":sub.ml",
)

# okapi:auto
ppx_ns_library(
    name = "#Sub_lib",
    submodules = [
        ":final",
        ":sub",
    ],
    visibility = ["//visibility:public"],
)

ocaml_module(
    name = "foo",
    deps_opam = [],
    opts = [],
    struct = ":foo.ml",
)

ocaml_module(
    name = "bar",
    deps_opam = [],
    opts = [],
    struct = ":bar.ml",
)

ocaml_ns_library(
    name = "#Sub_extra_lib",
    submodules = [
        ":foo",
        ":bar",
    ],
    visibility = ["//visibility:public"],
)
`

func checkFile(t *testing.T, ws string, target string, path... string) {
  rel := filepath.Join(path...)
  file := filepath.Join(strings.TrimSpace(ws), rel)
  bytes, err1 := ioutil.ReadFile(file)
  if err1 != nil { t.Fatal(err1) }
  content := string(bytes)
  if content != target { t.Fatal(rel + " doesn't match:\n" + content) }
}

func TestBuild(t *testing.T) {
  if err := bazel_testing.RunBazel("run", "//:gazelle"); err != nil { t.Fatal(err) }
  output, err := bazel_testing.BazelOutput("info", "workspace")
  ws := string(output[:])
  if err != nil { t.Fatal(err) }
  checkFile(t, ws, aBuildTarget, "a", "BUILD.bazel")
  checkFile(t, ws, subBuildTarget, "a", "sub", "BUILD.bazel")
}

func TestMain(m *testing.M) {
  bazel_testing.TestMain(m, testArgs)
}
