#!/bin/sh

### Compile and link statically
CGO_ENABLED=0 GOOS=linux go build -o check_xmpp -a -ldflags '-extldflags "-static" -w -s' check_xmpp.go
