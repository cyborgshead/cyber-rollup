#!/bin/bash

# Test script for EVM integration in Rollkit server

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Default JSON-RPC endpoint
ENDPOINT="http://127.0.0.1:8545"

# Check if jq is installed
if ! command -v jq &> /dev/null; then
    echo -e "${RED}Error: jq is not installed. Please install jq to run this script.${NC}"
    echo "On macOS: brew install jq"
    echo "On Ubuntu: apt-get install jq"
    exit 1
fi

# Function to make JSON-RPC request
function jsonrpc_request() {
    local method=$1
    local params=$2
    local result=$(curl -s -X POST -H "Content-Type: application/json" --data "{\"jsonrpc\":\"2.0\",\"method\":\"$method\",\"params\":$params,\"id\":1}" $ENDPOINT)
    echo $result
}

# Function to test a JSON-RPC method
function test_method() {
    local method=$1
    local params=$2
    local description=$3

    echo -e "\nTesting $description ($method)..."
    local response=$(jsonrpc_request "$method" "$params")

    if echo $response | jq -e '.error' > /dev/null; then
        echo -e "${RED}Error:${NC} $(echo $response | jq -r '.error.message')"
        return 1
    elif echo $response | jq -e '.result' > /dev/null; then
        echo -e "${GREEN}Success:${NC} $(echo $response | jq -r '.result')"
        return 0
    else
        echo -e "${RED}Error: Invalid response${NC}"
        echo $response
        return 1
    fi
}

# Main function
function main() {
    echo "=== EVM Integration Test ==="
    echo "JSON-RPC Endpoint: $ENDPOINT"

    # Test basic methods
    test_method "web3_clientVersion" "[]" "Client Version"
    test_method "net_version" "[]" "Network ID"
    test_method "eth_chainId" "[]" "Chain ID"
    test_method "eth_blockNumber" "[]" "Block Number"
    test_method "eth_gasPrice" "[]" "Gas Price"

    # Test account methods
    test_method "eth_accounts" "[]" "Accounts"

    # Test more complex methods
    test_method "eth_getBlockByNumber" "[\"latest\", false]" "Latest Block"

    echo -e "\n=== Test Complete ==="
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -e|--endpoint)
            ENDPOINT="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo "Options:"
            echo "  -e, --endpoint URL    JSON-RPC endpoint (default: http://localhost:8545)"
            echo "  -h, --help            Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Run the main function
main
