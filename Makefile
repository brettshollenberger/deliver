# vim: set ts=4 sw=4 tw=99 noet:

.PHONY: build install default all test

test:
	go test ./... -ginkgo.noColor

build:
	go build $(LDARGS)

install:
	go install $(LDARGS)

default: all
