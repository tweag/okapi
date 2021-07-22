load("@obazl_rules_ocaml//ocaml:repositories.bzl", "ocaml_dependencies")
load("@io_tweag_rules_nixpkgs//nixpkgs:nixpkgs.bzl", "nixpkgs_git_repository")
load("@io_tweag_rules_nixpkgs//nixpkgs:toolchains/go.bzl", "nixpkgs_go_configure")
load("@io_bazel_rules_go//go:deps.bzl", "go_rules_dependencies")
load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")
load("@bazel_gazelle//:deps.bzl", "go_repository")


def okapi_setup():
    ocaml_dependencies()
    nixpkgs_git_repository(
        name = "nixpkgs",
        revision = "21.05",
    )
    nixpkgs_go_configure(repository = "@nixpkgs")
    go_rules_dependencies()
    gazelle_dependencies(go_repository_default_config = "@//:WORKSPACE.bazel")
    go_repository(
        name = "com_github_chewxy_sexp",
        importpath = "github.com/chewxy/sexp",
        version = "v0.0.0-20181223234510-461851156c0f",
        sum = "h1:k5/iMSROZYPHPVKaWy5SrQeUxtCoSUsdRNJMwZAgdaI=",
    )
