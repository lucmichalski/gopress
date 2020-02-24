#!/usr/bin/env bash

# build views into binary and then deploy
echo "===== Generating assets file ======="
go-assets-builder views -o assets.go

env GOOS=linux GOARCH=amd64 go build -tags 'bindatafs' -o gopress-gin .

ssh -l root gopresssvr "systemctl stop gopress.service; systemctl status gopress.service; rm /home/apps/gopress/gopress-gin"
scp gopress-gin root@homefbase:/home/apps/gopress/

ssh -l root gopresssvr "systemctl start gopress.service; systemctl status gopress.service;"

echo "Cleaning Up"
rm gopress-gin

echo "Finshed build/deploy"
