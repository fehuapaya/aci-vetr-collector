#!/bin/bash

echo ============================== Test
go test

echo ============================== Build
gox -os="linux darwin windows" -arch="amd64" -output="aci-collector.{{.OS}}" -verbose ./...

echo ============================== Done
