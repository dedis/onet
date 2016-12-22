#!/usr/bin/env bash

DBG_TEST=2
DBG_APP=2
. $GOPATH/src/github.com/dedis/onet/app/libtest.sh

main(){
    startTest
    buildCothority import.go
#    test Build
#    test Status
    test Check
    stopTest
}

testCheck(){
	runCoBG 1 2
	testOK runCl check group.toml
	testOK runCl check -d group.toml
	cat group.toml <( tail -n 4 co3/group.toml ) > groupfail.toml
	testFail runCl check groupfail.toml
	runCoBG 3
	testOK runCl check -d groupfail.toml
}

testStatus(){
    runCoBG 1 2
    testOut "Running network"
    testGrep "Available_Services" runCl status group.toml
    testGrep "Available_Services" runCl status group.toml
}

testBuild(){
    testOK runCl --help
}

runCl(){
    dbgRun ./status -d $DBG_APP $@
}

main
