#!/bin/bash
rm -rf ./chartstorage
rm -rf ./temporary
rm -rf ./ca.crt
rm -rf ./ca.key
rm -rf ./ca.srl
rm -rf ./cacert.pem
rm -rf ./server.crt
rm -rf ./server.csr
rm -rf ./server.key
# badPid=$(netstat -vanp tcp | grep 8443| awk '{print $7}' | awk -F/ '{print $1}' | head -1)
# echo $badPid
# kill -9 $badPid
# kill -9 $(lsof -t -i:8443)