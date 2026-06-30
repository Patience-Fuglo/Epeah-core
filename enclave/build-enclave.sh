#!/bin/bash
set -euo pipefail

echo "==========================================="
echo " ARBITER NITRO ENCLAVE BUILD PIPELINE"
echo "==========================================="

echo "[1/3] Building production container image (enclave features enabled)..."
docker build -t arbiter-production-node:latest -f enclave/Dockerfile.prod ..

echo "[2/3] Converting container to Nitro Enclave Image File (.eif)..."
nitro-cli build-enclave \
    --docker-uri arbiter-production-node:latest \
    --output-file arbiter-core.eif

echo "[3/3] Launching enclave with dedicated CPU cores and memory..."
nitro-cli run-enclave \
    --eif-path arbiter-core.eif \
    --cpu-count 2 \
    --memory 4096 \
    --enclave-cid 16

echo ""
echo "==========================================="
echo " ENCLAVE DEPLOYED SUCCESSFULLY"
echo " CID: 16 | vSock Port: 5005"
echo " ARBITER_ENV=PROD"
echo " Verify: nitro-cli describe-enclaves"
echo "==========================================="
