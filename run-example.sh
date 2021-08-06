#!/usr/bin/env bash

cd example/project-$1
shift
bazel $*
