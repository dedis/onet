#!/usr/bin/env bash

# Usage:
#   ./test [options]
# Options:
#   -b   re-builds bcadmin package

DBG_TEST=2
DBG_DA=1

. "../app/libtest.sh"

DUMMY_DB="01d0fabd251fcbbe2b93b4b927b26ad2a1a99077152e45ded1e678afa45dbec5.db"

main(){
    startTest
    go build -o dummy/dummy ./dummy
    runDummy --save
    run testDB
    stopTest
}

testDB(){
    runDA --help
}

runDA(){
  dbgRun ./dbadmin --debug ${DBG_DA} "$@"
}

runDummy(){
  dummy/dummy $@
}
