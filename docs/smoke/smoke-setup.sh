#!/bin/bash

set -x

pwd
cd thornode

pwd

docker rm -f $(docker ps -a -q)

# # If the reset-mocknet-standalone can not delete the following files
# # add this rm to remove all files from ~/.thornode/standalone
# sudo rm -r $HOME/.thornode/standalone/.b* $HOME/.thornode/standalone/.t*

make -C build/docker reset-mocknet-standalone

sleep 1

docker stop btcq-indexer
docker stop timescale-db

sleep

cd ../btcq-indexer
pwd

docker-compose up -d pgtest

sleep 1

go run cmd/btcq-indexer/main.go config/config.json
