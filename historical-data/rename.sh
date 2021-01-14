#!/bin/sh

# remove the symlinks first, we'll recreate them later
rm common/*

# find and rename all the historical files
find . -name *testgrid* -exec rename  -- "-grid=old&show-stale" "-show-stale" {} \;

# recreate the symlinks
pushd common
for f in `ls -d ../4*`; do
  ln -s $f/* . 
done
popd

