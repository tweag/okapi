load(
    "@bazel_gazelle//:def.bzl",
    "DEFAULT_LANGUAGES",
    "gazelle",
    "gazelle_binary",
)

def generate():
    gazelle_binary(
        name = "gazelle_binary",
        languages = DEFAULT_LANGUAGES + ["@okapi//lang"],
    )
    gazelle(
        name = "gazelle",
        gazelle = "//:gazelle_binary",
    )
