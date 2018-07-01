#!/bin/bash

# Use with `VER=0.0.X ./build.sh`
[ -z "$BETAVER" ] && echo "Please use with a version env BETAVER=0.0.x-beta" && exit 1

rm -rf ./$BETAVER
mkdir $BETAVER
GOOS=linux GOARCH=amd64 go build -o ./$BETAVER/salienbot-$BETAVER-linux-x64 .
GOOS=linux GOARCH=386 go build -o ./$BETAVER/salienbot-$BETAVER-linux-x86 .
GOOS=darwin GOARCH=amd64 go build -o ./$BETAVER/salienbot-$BETAVER-darwin .
GOOS=windows GOARCH=amd64 go build -o ./$BETAVER/salienbot-$BETAVER-win64.exe .
GOOS=windows GOARCH=386 go build -o ./$BETAVER/salienbot-$BETAVER-win32.exe .
GOOS=linux GOARCH=arm go build -o ./$BETAVER/salienbot-$BETAVER-linux-arm .
GOOS=linux GOARCH=arm64 go build -o ./$BETAVER/salienbot-$BETAVER-linux-arm64 .

docker build --label "Version=$BETAVER" -t azusa0127/salienbot-go:$BETAVER . && docker push azusa0127/salienbot-go:$BETAVER
docker build --label "Version=$BETAVER" -t azusa0127/salienbot-go:latest .
