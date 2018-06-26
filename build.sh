#!/bin/bash

# Use with `VER=0.0.X ./build.sh`
[ -z "$VER" ] && echo "Please use with a version env VER=0.0.x" && exit 1

rm -rf ./$VER
mkdir $VER
GOOS=linux GOARCH=amd64 go build -o ./$VER/salienbot-$VER-linux-x64 .
GOOS=linux GOARCH=386 go build -o ./$VER/salienbot-$VER-linux-x86 .
GOOS=darwin GOARCH=amd64 go build -o ./$VER/salienbot-$VER-darwin .
GOOS=windows GOARCH=amd64 go build -o ./$VER/salienbot-$VER-win64.exe .
GOOS=windows GOARCH=386 go build -o ./$VER/salienbot-$VER-win32.exe .
GOOS=linux GOARCH=arm go build -o ./$VER/salienbot-$VER-linux-arm .
GOOS=linux GOARCH=arm64 go build -o ./$VER/salienbot-$VER-linux-arm64 .

docker build -t azusa0127/salienbot-go:$VER . && docker push azusa0127/salienbot-go:$VER
docker build -t azusa0127/salienbot-go:latest .
