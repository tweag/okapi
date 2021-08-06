package virtual_test

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
-- virt/virt.mli --
val base : int
-- virt/dune --
(library
  (name virt)
  (public_name virt)
  (virtual_modules virt)
)
-- impl1/virt.ml --
let base = 21
-- dep/dune --
(library
  (name dep)
  (public_name dep)
  (libraries virt)
  )
-- dep/dep.mli --
val number : int -> int
-- dep/dep.ml --
let number a = Virt.base + a
-- impl1/dune --
(library
 (name impl1)
 (public_name impl1)
 (implements virt)
 (modules virt)
)
-- impl2/virt.ml --
let base = 63
-- impl2/dune --
(library
 (name impl2)
 (public_name impl2)
 (implements virt)
 (modules virt)
-- exe/dune --
(executable
 (name main)
 (public_name exe)
 (libraries dep impl2)
  )
)
-- exe/main.ml --
print_endline ("number: " ^ string_of_int (Dep.number 21))
`,
  WorkspaceSuffix: `
load("@okapi//bzl:deps.bzl", "okapi_deps")
okapi_deps()

load("@okapi//bzl:setup.bzl", "okapi_setup_legacy")
okapi_setup_legacy()
`,
}

// TODO maybe not create a library rule?
const virtBuildTarget = `
load("@obazl_rules_ocaml//ocaml:rules.bzl", "ocaml_ns_library", "ocaml_signature")

# okapi:virt virt
ocaml_signature(
    name = "virt_sig",
    src = ":virt.mli",
    opts = ["-no-keep-locs"],
    visibility = ["//visibility:public"],
)

# okapi:auto
# okapi:public_name virt
ocaml_ns_library(
    name = "#Virt",
    submodules = [],
    visibility = ["//visibility:public"],
)
`

const impl1BuildTarget = `
load("@obazl_rules_ocaml//ocaml:rules.bzl", "ocaml_module", "ocaml_ns_library")

# okapi:implements virt
ocaml_module(
    name = "virt",
    opts = ["-no-keep-locs"],
    sig = "//virt:virt_sig",
    struct = ":virt.ml",
)

# okapi:implements virt
# okapi:implementation impl1
# okapi:public_name impl1
ocaml_ns_library(
    name = "#Impl1",
    submodules = [":virt"],
    visibility = ["//visibility:public"],
)
`

const impl2BuildTarget = `
load("@obazl_rules_ocaml//ocaml:rules.bzl", "ocaml_module", "ocaml_ns_library")

# okapi:implements virt
ocaml_module(
    name = "virt",
    opts = ["-no-keep-locs"],
    sig = "//virt:virt_sig",
    struct = ":virt.ml",
)

# okapi:implements virt
# okapi:implementation impl2
# okapi:public_name impl2
ocaml_ns_library(
    name = "#Impl2",
    submodules = [":virt"],
    visibility = ["//visibility:public"],
)
`

const depBuildTarget = `
load("@obazl_rules_ocaml//ocaml:rules.bzl", "ocaml_module", "ocaml_ns_library", "ocaml_signature")

ocaml_signature(
    name = "dep_sig",
    src = ":dep.mli",
    deps = [
        "//virt:#Virt",
        "//virt:virt_sig",
    ],
)

ocaml_module(
    name = "dep",
    sig = ":dep_sig",
    struct = ":dep.ml",
    deps = [
        "//virt:#Virt",
        "//virt:virt_sig",
    ],
)

# okapi:auto
# okapi:public_name dep
ocaml_ns_library(
    name = "#Dep",
    submodules = [":dep"],
    visibility = ["//visibility:public"],
)
`

const exeBuildTarget = `
load("@obazl_rules_ocaml//ocaml:rules.bzl", "ocaml_executable", "ocaml_module")

ocaml_module(
    name = "main",
    struct = ":main.ml",
    deps = [
        "//dep:#Dep",
        "//impl2:#Impl2",
    ],
)

# okapi:auto
# okapi:public_name exe
ocaml_executable(
    name = "exe",
    main = "main",
    visibility = ["//visibility:public"],
    deps = ["//impl2:#Impl2"],
)
`

func checkFile(t *testing.T, ws string, target string, path... string) {
  trimmedTarget := strings.TrimSpace(target)
  rel := filepath.Join(path...)
  file := filepath.Join(strings.TrimSpace(ws), rel)
  bytes, err1 := ioutil.ReadFile(file)
  if err1 != nil { t.Fatal(err1) }
  content := strings.TrimSpace(string(bytes))
  if content != trimmedTarget {
    t.Fatal(rel + " doesn't match:\n" + content + "\n\n------------------- target:\n" + trimmedTarget)
  }
}

func run(t *testing.T, cmd... string) string {
  if output, err := bazel_testing.BazelOutput(cmd...); err != nil {
    t.Fatal(err)
    return ""
  } else {
    return string(output)
  }
}

func TestVirtual(t *testing.T) {
  ws := run(t, "info", "workspace")
  run(t, "run", "//:gazelle")
  checkFile(t, ws, virtBuildTarget, "virt", "BUILD.bazel")
  checkFile(t, ws, impl1BuildTarget, "impl1", "BUILD.bazel")
  checkFile(t, ws, impl2BuildTarget, "impl2", "BUILD.bazel")
  checkFile(t, ws, depBuildTarget, "dep", "BUILD.bazel")
  checkFile(t, ws, exeBuildTarget, "exe", "BUILD.bazel")
}

func TestMain(m *testing.M) {
  bazel_testing.TestMain(m, testArgs)
}
