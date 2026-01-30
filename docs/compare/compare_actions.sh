#!/bin/bash

set -eu -o pipefail

midgard_old=https://midgard.thorchain.info/v2/actions
midgard_new=http://localhost:8080/v2/actions

mkdir -p new old

function fetch() {
    params=$1
    name=$2
    curl "$midgard_new?$params" | jq -c '.actions[]' | sort > new/$name
    curl "$midgard_old?$params" | jq -c '.actions[]' | sort > old/$name
}

fetch 'limit=50' base
fetch 'offset=200&limit=50' off200
fetch 'type=switch' switch
fetch 'type=refund' refund
fetch 'type=donate' donate
fetch 'type=withdraw' withdraw
fetch 'type=swap' swap
fetch 'type=addLiquidity' addLiquidity
