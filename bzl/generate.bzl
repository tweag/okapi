load(
    "@bazel_gazelle//:def.bzl",
    "gazelle",
    "gazelle_binary",
)

def generate():
    gazelle_binary(
        name = "gazelle_binary",
        languages = ["@okapi//lang"],
    )
    gazelle(
        name = "gazelle",
        gazelle = "//:gazelle_binary",
    )
