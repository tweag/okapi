package no_sig_test

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
-- lib/sig.ml --
module type A = sig
  val v : int
end
-- lib/prog.mli --
include Sig.A
-- lib/prog.ml --
let v = 5
-- lib/dune --
(library
  (name lib)
  (public_name lib)
)
`,
	WorkspaceSuffix: `
load("@okapi//bzl:deps.bzl", "okapi_deps")
okapi_deps()

load("@okapi//bzl:setup.bzl", "okapi_setup_legacy")
okapi_setup_legacy()
`,
}

const libBuildTarget = `
load("@obazl_rules_ocaml//ocaml:rules.bzl", "ocaml_module", "ocaml_ns_library", "ocaml_signature")

ocaml_signature(
    name = "prog__sig",
    src = ":prog.mli",
    deps = [":sig"],
)

ocaml_module(
    name = "prog",
    sig = ":prog__sig",
    struct = ":prog.ml",
    deps = [":sig"],
)

ocaml_module(
    name = "sig",
    struct = ":sig.ml",
)

# okapi:auto
# okapi:public_name lib
ocaml_ns_library(
    name = "#Lib",
    submodules = [
        ":prog",
        ":sig",
    ],
    visibility = ["//visibility:public"],
)
`

func checkFile(t *testing.T, ws string, target string, path ...string) {
	trimmedTarget := strings.TrimSpace(target)
	rel := filepath.Join(path...)
	file := filepath.Join(strings.TrimSpace(ws), rel)
	bytes, err1 := ioutil.ReadFile(file)
	if err1 != nil {
		t.Fatal(err1)
	}
	content := strings.TrimSpace(string(bytes))
	if content != trimmedTarget {
		t.Fatal(rel + " doesn't match:\n\n" + content + "\n\n------------------- target:\n\n" + trimmedTarget)
	}
}

func run(t *testing.T, cmd ...string) string {
	if output, err := bazel_testing.BazelOutput(cmd...); err != nil {
		t.Fatal(err)
		return ""
	} else {
		return string(output)
	}
}

func TestVirtual(t *testing.T) {
	ws := run(t, "info", "workspace")
	run(t, "run", "//:gazelle", "--", "--library")
	checkFile(t, ws, libBuildTarget, "lib", "BUILD.bazel")
}

func TestMain(m *testing.M) {
	bazel_testing.TestMain(m, testArgs)
}
