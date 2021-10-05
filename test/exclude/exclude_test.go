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
-- lib/m1.ml --
-- lib/m2.ml --
-- lib/m3.ml --
-- lib/m4.ml --
-- lib/dune --
(library
  (name lib1)
  (modules (:standard \ m2 m3))
)
(library
  (name lib2)
  (modules m1)
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
load("@obazl_rules_ocaml//ocaml:rules.bzl", "ocaml_module", "ocaml_ns_library")

ocaml_module(
    name = "m4",
    struct = ":m4.ml",
)

ocaml_module(
    name = "m1",
    struct = ":m1.ml",
)

# okapi:auto
# okapi:public_name lib1
ocaml_ns_library(
    name = "#Lib1",
    submodules = [":m4"],
    visibility = ["//visibility:public"],
)

# okapi:public_name lib2
ocaml_ns_library(
    name = "#Lib2",
    submodules = [":m1"],
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
