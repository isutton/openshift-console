#!/bin/bash
cd ../testdata/
curl --cert-type PEM   --cacert ../cacert.pem  --data-binary "@mychart-0.1.0.tgz" https://localhost:8443/api/charts
#push mariadb chart
curl --cert-type PEM   --cacert ../cacert.pem --data-binary "@mariadb-7.3.5.tgz" https://localhost:8443/api/charts