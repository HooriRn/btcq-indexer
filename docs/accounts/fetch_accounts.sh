#!/bin/bash
# server=https://thornode.ninerealms.com
set -euo pipefail
fetch()
{
	server=$1
	endpoint=$2
	page_key=$3

	curl --insecure --data-urlencode "pagination.key=$page_key" $server/$endpoint
}
itemcount()
{
	input=$1
	
 	jq ".accounts[].address" $input | wc -l
}
page()
{
	input=$1
	jq ".pagination.next_key" $1 | tr -d '"'
}


server=$1
endpoint=cosmos/auth/v1beta1/accounts
page_key=$2
offset=0
count=-1
while [[ $count != 0 ]]
do
	output=accounts_$offset
	fetch $server $endpoint $page_key > $output
	count=$( itemcount $output )
	offset=$[offset+count]
	page_key=$( page $output )
	if [[ "$page_key" = "null" ]]; then
		exit 0
	fi
	sleep 10
done
