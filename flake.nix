{
  description = "Gazelle Extension for OBazl";

  inputs.obazl.url = github:tek/rules_ocaml;

  outputs = { obazl, ... }:
  let
    depsOpam = [
      { name = "codept"; version = "0.11.0"; }
      { name = "angstrom"; version = "0.15.0"; }
    ];

  in obazl.flakes { inherit depsOpam; };
}
