#!/bin/bash
openssl genrsa -out ca.key 2048
openssl req -new -x509 -days 365 -key ca.key -subj  "/C=DE/ST=NRW/L=Berlin/O=My Inc/OU=DevOps/CN=localhost/emailAddress=dev@www.example.com"  -out ca.crt

openssl req -newkey rsa:2048 -nodes -keyout server.key -subj  "/C=DE/ST=NRW/L=Berlin/O=My Inc/OU=DevOps/CN=localhost/emailAddress=dev@www.example.com" -out server.csr

openssl x509 -req -extfile <(printf "subjectAltName=DNS:localhost") -days 365 -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt

#openssl x509 -req -extfile altsubj.ext -keyout key.pem -out cert.pem -subj "/C=DE/ST=NRW/L=Berlin/O=My Inc/OU=DevOps/CN=localhost/emailAddress=dev@www.example.com" 
python3 chartProxy_test.py &
sleep 1
echo quit | openssl s_client -showcerts -servername localhost -connect localhost:8080 > cacert.pem

# kubectl delete secret my-repo -n test 
# kubectl create secret generic my-repo  --from-file=tls.key=./server.key  --from-file=tls.crt=./server.crt  -n test
#kubectl create cm  my-repo  --from-file=ca-bundle.crt=./cacert.pem  -n openshift-config
# curl --cert-type PEM   --cacert /Users/kmamgain/Projects/console/pkg/helm/chartproxy/cacert.pem  --data-binary "@my-chart-0.1.0.tgz" https://localhost:8080/api/charts
curl --cert ./server.crt  --key ./server.key --cacert ./cacert.pem  https://localhost:8080/

kill %1