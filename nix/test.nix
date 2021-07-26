{ pkgs, }:
with pkgs.lib;
let
  filterProject = path: type:
  let
    n = baseNameOf path;
    isBuild = n == "BUILD.bazel";
    isRoot = baseNameOf (dirOf path) == "project-1";
  in !(isRoot && n == "main") && (isBuild && isRoot) || !isBuild;

  project1 = builtins.filterSource filterProject ../test/project-1;

in {
  test = pkgs.writeScript "okapi-tests" ''
    #!${pkgs.zsh}/bin/zsh
    base=$PWD
    work=$base/test/temp
    rm -rf $work
    mkdir -p $work
    ${pkgs.rsync}/bin/rsync -r --exclude '*/BUILD.bazel' --exclude '/main' $base/test/project-1/ $work/
    cd $work
    bazel run //:gazelle
    bazel build //a/...
  '';
}
