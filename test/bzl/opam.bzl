load("@obazl_rules_ocaml//ocaml:providers.bzl", "BuildConfig", "OpamConfig")

opam_pkgs = {
    "ocaml": [],
}

opam = OpamConfig(
    version = "2.0",
    builds = {
        "4.10": BuildConfig(
            default = True,
            switch = "4.10",
            compiler = "4.10",
            packages = opam_pkgs,
            install = True,
        ),
    },
)
