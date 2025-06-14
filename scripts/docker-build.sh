#!/bin/bash

# Docker-based build script for lambda-nat-proxy
# This script builds the entire project using Docker, eliminating host dependencies

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Build directory
BUILD_DIR="build"

echo -e "${BLUE}🐳 Building lambda-nat-proxy using Docker...${NC}"
echo -e "${YELLOW}📦 This includes all dependencies (Node.js, Go, etc.)${NC}"

# Create build directory
mkdir -p ${BUILD_DIR}

# Build using Docker
# Detect host platform for building native binary
HOST_OS=$(uname -s | tr '[:upper:]' '[:lower:]')
HOST_ARCH=$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')

echo -e "${BLUE}🔨 Building Docker image for ${HOST_OS}/${HOST_ARCH}...${NC}"

# Build the Docker image with host platform targeting
docker build \
    --build-arg TARGETOS="${HOST_OS}" \
    --build-arg TARGETARCH="${HOST_ARCH}" \
    -f Dockerfile.build \
    -t lambda-nat-proxy-builder \
    .

# Extract binaries from the built image
echo -e "${BLUE}📤 Extracting built binaries...${NC}"

# Create a temporary container and copy files
CONTAINER_ID=$(docker create lambda-nat-proxy-builder)

# Copy main binary
docker cp ${CONTAINER_ID}:/root/lambda-nat-proxy ./${BUILD_DIR}/lambda-nat-proxy

# Copy lambda bootstrap
docker cp ${CONTAINER_ID}:/root/assets/bootstrap ./${BUILD_DIR}/bootstrap

# Clean up container
docker rm ${CONTAINER_ID}

# Make binaries executable
chmod +x ${BUILD_DIR}/lambda-nat-proxy
chmod +x ${BUILD_DIR}/bootstrap

echo -e "${GREEN}✅ Build complete!${NC}"
echo -e "${GREEN}📁 Binaries available in: ${BUILD_DIR}/${NC}"
echo ""
echo -e "${BLUE}Built artifacts:${NC}"
echo -e "  • ${BUILD_DIR}/lambda-nat-proxy (CLI with embedded dashboard and Lambda)"
echo -e "  • ${BUILD_DIR}/bootstrap (Lambda function)"
echo ""
echo -e "${YELLOW}💡 To run: ./${BUILD_DIR}/lambda-nat-proxy --help${NC}"

# Optional: Clean up Docker image
if [[ "${CLEANUP:-yes}" == "yes" ]]; then
    echo -e "${BLUE}🧹 Cleaning up Docker image...${NC}"
    docker rmi lambda-nat-proxy-builder
fi