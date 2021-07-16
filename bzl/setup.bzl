load("@obazl_rules_ocaml//ocaml:repositories.bzl", "ocaml_dependencies")
load("@io_tweag_rules_nixpkgs//nixpkgs:nixpkgs.bzl", "nixpkgs_git_repository")
load("@io_tweag_rules_nixpkgs//nixpkgs:toolchains/go.bzl", "nixpkgs_go_configure")
load("@io_bazel_rules_go//go:deps.bzl", "go_rules_dependencies")
load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")

def okapi_setup():
    ocaml_dependencies()
    nixpkgs_git_repository(
        name = "nixpkgs",
        revision = "21.05",
    )
    nixpkgs_go_configure(repository = "@nixpkgs")
    go_rules_dependencies()
    gazelle_dependencies()
