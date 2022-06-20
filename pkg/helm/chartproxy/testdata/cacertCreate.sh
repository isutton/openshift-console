#!/bin/bash
echo quit | openssl s_client -showcerts -servername localhost -connect localhost:8443 > cacert.pem