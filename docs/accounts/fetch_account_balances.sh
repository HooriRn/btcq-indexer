#!/bin/bash
set -euo pipefail
address=$1
height="x-cosmos-block-height: $2"
server=${3:-https://thornode.ninerealms.com}
endpoint="cosmos/bank/v1beta1/balances"
curl --insecure ${server}/${endpoint}/${address} -H "${height}"
