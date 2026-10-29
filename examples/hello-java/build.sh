#!/bin/sh
# Builds hello.jar (Java 17 bytecode). Needs a JDK 17+: brew install openjdk
set -eu
cd "$(dirname "$0")"
rm -rf out && mkdir out
javac --release 17 -d out Hello.java
printf 'Main-Class: Hello\n' > out/MANIFEST.MF
jar --create --file hello.jar --manifest out/MANIFEST.MF -C out Hello.class
rm -rf out
echo "built $(pwd)/hello.jar"
