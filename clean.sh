#!/bin/bash

for subdir in $@; do
    pushd $subdir
    make clean
    popd
done
