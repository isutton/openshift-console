#!/bin/bash
curl https://raw.githubusercontent.com/helm/chartmuseum/main/scripts/get-chartmuseum | bash --wait

chartmuseum --debug --port=8080 \
  --storage="local" \
  --storage-local-rootdir="./chartstorage" \
  --tls-cert=./server.crt --tls-key=./server.key 
