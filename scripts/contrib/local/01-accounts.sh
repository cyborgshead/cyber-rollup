#!/bin/bash
set -o errexit -o nounset -o pipefail

BASE_ACCOUNT=$(cyber keys show validator -a --keyring-backend=test)
cyber q auth account "$BASE_ACCOUNT" -o json | jq

echo "## Add new account"
cyber keys add fred --keyring-backend=test

echo "## Check balance"
NEW_ACCOUNT=$(cyber keys show fred -a --keyring-backend=test)
cyber q bank balances "$NEW_ACCOUNT" -o json || true

echo "## Transfer tokens"
cyber tx bank send validator "$NEW_ACCOUNT" 1ustake --gas 1000000 -y --chain-id=testing --node=http://localhost:26657 -b sync -o json --keyring-backend=test | jq

echo "## Check balance again"
cyber q bank balances "$NEW_ACCOUNT" -o json | jq
