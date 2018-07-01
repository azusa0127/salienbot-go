#!/bin/bash

# Use with `VER=0.0.X ./build.sh`
[ -z "$VER-beta" ] && echo "Please use with a version env VER=0.0.x" && exit 1

rm -rf ./$VER-beta
mkdir $VER-beta
GOOS=linux GOARCH=amd64 go build -o ./$VER-beta/salienbot-$VER-beta-linux-x64 .
GOOS=linux GOARCH=386 go build -o ./$VER-beta/salienbot-$VER-beta-linux-x86 .
GOOS=darwin GOARCH=amd64 go build -o ./$VER-beta/salienbot-$VER-beta-darwin .
GOOS=windows GOARCH=amd64 go build -o ./$VER-beta/salienbot-$VER-beta-win64.exe .
GOOS=windows GOARCH=386 go build -o ./$VER-beta/salienbot-$VER-beta-win32.exe .
GOOS=linux GOARCH=arm go build -o ./$VER-beta/salienbot-$VER-beta-linux-arm .
GOOS=linux GOARCH=arm64 go build -o ./$VER-beta/salienbot-$VER-beta-linux-arm64 .

docker build --label "Version=$VER-beta" -t azusa0127/salienbot-go:$VER-beta . && docker push azusa0127/salienbot-go:$VER-beta
docker build --label "Version=$VER-beta" -t azusa0127/salienbot-go:latest .
