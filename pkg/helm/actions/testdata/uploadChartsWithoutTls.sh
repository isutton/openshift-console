#!/bin/bash
#push mariadb chart
cd ../testdata/
curl  --data-binary "@mariadb-7.3.5.tgz" http://localhost:8080/api/charts
curl  --data-binary "@influxdb-3.0.2.tgz" http://localhost:8080/api/charts