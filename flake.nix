{
  description = "Gazelle Extension for OBazl";

  inputs.obazl.url = "github:tek/rules_ocaml";

  outputs = { obazl, ... }:
    let depsOpam = [ "codept" ];

    in obazl.systems {
      inherit depsOpam;
      extraInputs = pkgs: [ pkgs.go ];
    };
}
