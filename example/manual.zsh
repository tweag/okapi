#!/usr/bin/env zsh

base=${0:h}
temp=$base/temp
out=$temp/out
pro=$base/project-2

rm -rf $temp
mkdir -p $out/{virt,dep,impl,exe}
rsync -r --exclude '_build/' --include '*/' --include '*.mli' --include '*.ml' --exclude '*' $pro/ $temp/
cd $temp

ns='module Virty = Virt__Virty'

print $ns > $out/virt/virt.ml-gen
print $ns > $out/impl/virt_impl.ml-gen
print 'module Deppy = Dep__Deppy' > $out/dep/dep.ml-gen

args=(-strict-sequence -strict-formats -short-paths -keep-locs -g -bin-annot -opaque -keep-locs -no-alias-deps)

compile()
{
  # print "ocamlc $@"
  ocamlc "${(@)args}" -c $@
}

opt()
{
  ocamlopt "${(@)args}" -c $@
}

intf()
{
  compile -intf $1/$2.mli -o $out/$1/$3.cmi $@[4,$]
}

impl()
{
  compile -intf-suffix .ml -impl $1/$2.ml -o $out/$1/$3.cmo $@[4,$]
}

implo()
{
  opt -intf-suffix .ml -impl $1/$2.ml -o $out/$1/$3.cmx $@[4,$]
}

compile -w -49 -o $out/virt/virt.cmo -c -impl $out/virt/virt.ml-gen
compile -w -49 -o $out/impl/virt_impl.cmo -c -impl $out/impl/virt_impl.ml-gen
compile -w -49 -o $out/dep/dep.cmo -c -impl $out/dep/dep.ml-gen

intf virt virty virt__Virty -I $out/virt
intf dep deppy dep__Deppy

# impl impl virty virt__Virty -I $out/virt
# impl dep deppy dep__Deppy -I $out/virt -I $out/dep
# compile exe/main.ml -o $out/exe/exe__Main.cmo -I $out/dep

implo impl virty virt__Virty -I $out/virt
implo dep deppy dep__Deppy -I $out/virt -I $out/dep
opt exe/main.ml -o $out/exe/exe__Main.cmo -I $out/dep

opt -w -49 -o $out/virt/virt.cmx -c -impl $out/virt/virt.ml-gen
opt -w -49 -o $out/impl/virt_impl.cmx -c -impl $out/impl/virt_impl.ml-gen
opt -w -49 -o $out/dep/dep.cmx -c -impl $out/dep/dep.ml-gen

ocamlopt -a -o $out/impl/impl.cmxa $out/impl/virt__Virty.cmx $out/virt/virt.cmx
ocamlopt -a -o $out/dep/dep.cmxa $out/dep/dep.cmx $out/dep/dep__Deppy.cmx
ocamlopt -shared -linkall -I impl -o $out/impl/impl.cmxs $out/impl/impl.cmxa
ocamlopt -shared -linkall -I dep -o $out/dep/dep.cmxs $out/dep/dep.cmxa

ocamlopt -o $out/exe/main $out/impl/impl.cmxa $out/dep/dep.cmxa $out/exe/exe__Main.cmx

tree $out

# ocamlobjinfo $out/virt/virt__Virty.cmi
ocamlobjinfo $out/impl/virt__Virty.cmx
ocamlobjinfo $out/dep/dep__Deppy.cmx

$out/exe/main

rm -rf $temp
