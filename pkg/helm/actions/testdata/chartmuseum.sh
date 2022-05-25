#!/bin/bash
curl https://raw.githubusercontent.com/helm/chartmuseum/main/scripts/get-chartmuseum | bash --wait
# helm repo update
# chartmuseum server running
#helm repo add chartmuseum http://localhost:8080 --cacert .././cacert.pem 
chartmuseum --debug --port=8443 \
  --storage="local" \
  --storage-local-rootdir="./chartstorage" \
  --tls-cert=./server.crt --tls-key=./server.key