#!/bin/bash

#Set Script Name variabl
SCRIPT=`basename ${BASH_SOURCE[0]}`

MODE="emulator"
TIMEOUT="60m"
# run go test with -v flag
VERBOSE=""
# run go test with -run flag
RUN_TESTS=""
FAILFAST=""
NAME=""

usage () {
  echo "Usage: $SCRIPT"
  echo "Optional command line arguments"
  echo "-m <string>  -- Testmode to run, EMULATOR or USB;"
  echo "-r <string>  -- Run test with -run flag"
  echo "-n <string>  -- Specific name for this test, affects coverage output files"
  echo "-v <boolean> -- Run test with -v flag"
  echo "-f <boolean> -- Run test with -failfast flag"
  exit 1
}

while getopts "h?m:r:n:uvfca" args; do
case $args in
    h|\?)
        usage;
        exit;;
    m ) MODE=${OPTARG};;
    r ) RUN_TESTS="-run ${OPTARG}";;
    n ) NAME="-${OPTARG}";;
    v ) VERBOSE="-v";;
    f ) FAILFAST="-failfast";;
  esac
done


set +e

HW_GO_INTEGRATION_TESTS=1 HW_GO_INTEGRATION_TEST_MODE=$MODE \
    go test ./src/cli/integration/... $FAILFAST -timeout=$TIMEOUT $VERBOSE $RUN_TESTS

TEST_FAIL=$?

exit $TEST_FAIL
