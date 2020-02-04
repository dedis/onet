#!/usr/bin/env bash

# Usage:
#   ./test [options]
# Options:
#   -b   re-builds bcadmin package

DBG_TEST=1
DBG_DA=1

. "../app/libtest.sh"

# This is constant as the 'dummy' creation uses always this db. It's the
# sha256(ed25519.base)
DUMMY_DB="01d0fabd251fcbbe2b93b4b927b26ad2a1a99077152e45ded1e678afa45dbec5.db"

main(){
    startTest
    go build ../dummy
    runDummy --save
    run testInspect
    run testExtract
    run testMerge
    stopTest
}

testInspect(){
    testFail runDA inspect
    testFail runDA inspect dummy.db
    testFail runDA inspect --source dummy.db
    testGrep foo runDA inspect ${DUMMY_DB}
    testReGrep bar
    testGrep foo runDA inspect --source ${DUMMY_DB}
    testReGrep bar
}

testExtract(){
  testFail runDA extract
  testFail runDA extract --source dummy.db
  testFail runDA extract --source ${DUMMY_DB}

  TMPDB="tmp.db"
  rm -f ${TMPDB}
  testOK runDA extract --source ${DUMMY_DB} --destination ${TMPDB} Bar
  testGrep Bar runDA inspect ${TMPDB}

  rm -f ${TMPDB}
  testOK runDA extract --source ${DUMMY_DB} --destination ${TMPDB} Bar.*
  testGrep Bar runDA inspect ${TMPDB}
  testReGrep Bar_barDB
  testReGrep Barversion

  rm -f ${TMPDB}
  testOK runDA extract --source ${DUMMY_DB} --destination ${TMPDB}
  testGrep Bar runDA inspect ${TMPDB}
  testGrep Foo runDA inspect ${TMPDB}
}

testMerge(){
  TMPDB="tmp.db"
  MERGEDB="merge.db"
  rm -f ${TMPDB} ${MERGEDB}

  testOK runDA extract --source ${DUMMY_DB} --destination ${MERGEDB} Bar.*
  testNGrep Foo runDA inspect ${MERGEDB}
  testFail runDA extract --source ${DUMMY_DB} --destination ${MERGEDB} Foo.*
  testOK runDA extract --source ${DUMMY_DB} --destination ${MERGEDB} \
      --overwrite Foo.*
  testGrep Foo runDA inspect ${MERGEDB}
  testReGrep Foo_fooDB
  testReGrep Fooversion
}

runDA(){
    ./dbadmin --debug ${DBG_DA} "$@"
}

runDummy(){
    ./dummy "$@"
}

main
