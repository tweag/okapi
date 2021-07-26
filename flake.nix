{
  description = "Gazelle Extension for OBazl";

  inputs = {
    nixpkgs.url = github:NixOS/nixpkgs/fbfb79400a08bf754e32b4d4fc3f7d8f8055cf94;
    flake-utils.url = github:numtide/flake-utils;
    obazl.url = github:tek/rules_ocaml;
  };

  outputs = { nixpkgs, flake-utils, obazl, ... }:
  let
    main = system:
    let
      pkgs = import nixpkgs { inherit system; };
      test = import nix/test.nix { inherit pkgs; };
      testInShell = pkgs.writeScript "test" "nix develop -c ${test.test}";
    in {
      devShell = (obazl.flakes {}).shell pkgs;
      apps = {
        test = {
          type = "app";
          program = "${testInShell}";
        };
      };
    };

  in flake-utils.lib.eachDefaultSystem main;
}
