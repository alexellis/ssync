#!/bin/sh

for f in bin/ssync*; do shasum -a 256 $f > $f.sha256; done
