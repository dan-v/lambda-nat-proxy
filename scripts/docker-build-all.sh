#!/bin/bash

# Multi-platform Docker build script for lambda-nat-proxy
# Builds binaries for multiple operating systems and architectures

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Build directory
BUILD_DIR="build"

# Platforms to build for (compatible with older bash)
PLATFORMS=(
    "linux-amd64:linux:amd64"
    "linux-arm64:linux:arm64"
    "darwin-amd64:darwin:amd64"
    "darwin-arm64:darwin:arm64"
    "windows-amd64:windows:amd64"
)

echo -e "${BLUE}🌍 Building lambda-nat-proxy for multiple platforms using Docker...${NC}"

# Check if buildx is available
BUILDX_AVAILABLE=false
if docker buildx version >/dev/null 2>&1; then
    echo -e "${GREEN}✅ Docker buildx detected - using advanced multi-platform build${NC}"
    BUILDX_AVAILABLE=true
else
    echo -e "${YELLOW}⚠️  Docker buildx not available - using sequential builds${NC}"
    echo -e "${YELLOW}💡 For faster builds, consider updating Docker to support buildx${NC}"
fi

# Create build directory
mkdir -p ${BUILD_DIR}

if [[ "$BUILDX_AVAILABLE" == "true" ]]; then
    # Use buildx for efficient multi-platform builds
    echo -e "${BLUE}🚀 Using Docker buildx for efficient multi-platform build...${NC}"
    
    # Create buildx builder if it doesn't exist
    docker buildx create --name multiarch-builder --use 2>/dev/null || docker buildx use multiarch-builder 2>/dev/null || true
    
    # Create multi-stage Dockerfile for cross-compilation
    cat > Dockerfile.multiarch << 'EOF'
FROM --platform=$BUILDPLATFORM node:18-alpine AS dashboard-builder
WORKDIR /app
COPY web/package*.json ./web/
WORKDIR /app/web
RUN npm ci --only=production
COPY web/ .
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS go-builder
ARG TARGETOS
ARG TARGETARCH
RUN apk add --no-cache git
WORKDIR /app

# Copy all source code first (needed for replace directive)
COPY . .
COPY --from=dashboard-builder /app/web/dist ./internal/dashboard/web/dist

# Fix the replace directive in lambda/go.mod to use absolute path
RUN sed -i 's|replace github.com/dan-v/lambda-nat-punch-proxy => ..|replace github.com/dan-v/lambda-nat-punch-proxy => /app|' lambda/go.mod

# Download dependencies (replace directive works now)
RUN go mod download && cd lambda && go mod download

# Build Lambda function (always Linux/amd64 for AWS)
RUN cd lambda && GOOS=linux GOARCH=amd64 go build -o ../cmd/lambda-nat-proxy/assets/bootstrap .

# Build main binary for target platform
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -installsuffix cgo -o lambda-nat-proxy ./cmd/lambda-nat-proxy

FROM scratch
COPY --from=go-builder /app/lambda-nat-proxy .
COPY --from=go-builder /app/cmd/lambda-nat-proxy/assets/bootstrap ./bootstrap
EOF

    # Build for each platform using buildx
    for platform_spec in "${PLATFORMS[@]}"; do
        IFS=':' read -r platform_key os arch <<< "$platform_spec"
        
        echo -e "${BLUE}🔨 Building for ${os}/${arch}...${NC}"
        
        binary_name="lambda-nat-proxy"
        if [[ "$os" == "windows" ]]; then
            binary_name="lambda-nat-proxy.exe"
        fi
        
        output_dir="${BUILD_DIR}/${platform_key}"
        mkdir -p "$output_dir"
        
        docker buildx build \
            --platform "${os}/${arch}" \
            --file Dockerfile.multiarch \
            --output "type=local,dest=${output_dir}" \
            .
        
        # Rename binary
        mv "${output_dir}/lambda-nat-proxy" "${output_dir}/${binary_name}" 2>/dev/null || true
        
        # Copy bootstrap (first time only)
        cp "${output_dir}/bootstrap" "${BUILD_DIR}/bootstrap" 2>/dev/null || true
        
        # Make executable (not needed for Windows)
        if [[ "$os" != "windows" ]]; then
            chmod +x "${output_dir}/${binary_name}"
        fi
        
        echo -e "${GREEN}  ✅ Built: ${output_dir}/${binary_name}${NC}"
    done
    
    rm -f Dockerfile.multiarch
    
else
    # Fallback: Build using standard Docker with GOOS/GOARCH
    echo -e "${BLUE}🔨 Using standard Docker builds with Go cross-compilation...${NC}"
    
    # Create standard Dockerfile for cross-compilation
    cat > Dockerfile.standard << 'EOF'
FROM node:18-alpine AS dashboard-builder
WORKDIR /app
COPY web/package*.json ./web/
WORKDIR /app/web
RUN npm ci --only=production
COPY web/ .
RUN npm run build

FROM golang:1.21-alpine AS go-builder
ARG TARGETOS
ARG TARGETARCH
RUN apk add --no-cache git
WORKDIR /app

# Copy all source code first (needed for replace directive)
COPY . .
COPY --from=dashboard-builder /app/web/dist ./internal/dashboard/web/dist

# Fix the replace directive in lambda/go.mod to use absolute path
RUN sed -i 's|replace github.com/dan-v/lambda-nat-punch-proxy => ..|replace github.com/dan-v/lambda-nat-punch-proxy => /app|' lambda/go.mod

# Download dependencies (replace directive works now)
RUN go mod download && cd lambda && go mod download

# Build Lambda function (always Linux/amd64 for AWS)
RUN cd lambda && GOOS=linux GOARCH=amd64 go build -o ../cmd/lambda-nat-proxy/assets/bootstrap .

# Build main binary for target platform
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -a -installsuffix cgo -o lambda-nat-proxy ./cmd/lambda-nat-proxy
EOF

    # Build for each platform
    for platform_spec in "${PLATFORMS[@]}"; do
        IFS=':' read -r platform_key os arch <<< "$platform_spec"
        
        echo -e "${BLUE}🔨 Building for ${os}/${arch}...${NC}"
        
        binary_name="lambda-nat-proxy"
        if [[ "$os" == "windows" ]]; then
            binary_name="lambda-nat-proxy.exe"
        fi
        
        output_dir="${BUILD_DIR}/${platform_key}"
        mkdir -p "$output_dir"
        
        # Build Docker image with target OS/ARCH
        docker build \
            --build-arg TARGETOS="${os}" \
            --build-arg TARGETARCH="${arch}" \
            --file Dockerfile.standard \
            --tag "lambda-nat-proxy-${platform_key}" \
            .
        
        # Extract binaries from built image
        CONTAINER_ID=$(docker create "lambda-nat-proxy-${platform_key}")
        docker cp "${CONTAINER_ID}:/app/lambda-nat-proxy" "${output_dir}/${binary_name}"
        docker cp "${CONTAINER_ID}:/app/cmd/lambda-nat-proxy/assets/bootstrap" "${output_dir}/bootstrap" 2>/dev/null || true
        docker rm "${CONTAINER_ID}"
        docker rmi "lambda-nat-proxy-${platform_key}"
        
        # Copy bootstrap (first time only)
        cp "${output_dir}/bootstrap" "${BUILD_DIR}/bootstrap" 2>/dev/null || true
        
        # Make executable (not needed for Windows)
        if [[ "$os" != "windows" ]]; then
            chmod +x "${output_dir}/${binary_name}"
        fi
        
        echo -e "${GREEN}  ✅ Built: ${output_dir}/${binary_name}${NC}"
    done
    
    rm -f Dockerfile.standard
fi

echo -e "${GREEN}🎉 Multi-platform build complete!${NC}"
echo -e "${GREEN}📁 Binaries available in: ${BUILD_DIR}/*/lambda-nat-proxy*${NC}"
echo ""
echo -e "${BLUE}Built platforms:${NC}"
for platform_spec in "${PLATFORMS[@]}"; do
    IFS=':' read -r platform_key os arch <<< "$platform_spec"
    binary_name="lambda-nat-proxy"
    if [[ "$os" == "windows" ]]; then
        binary_name="lambda-nat-proxy.exe"
    fi
    echo -e "  • ${BUILD_DIR}/${platform_key}/${binary_name}"
done
echo -e "  • ${BUILD_DIR}/bootstrap (Lambda function - Linux only)"

# Detect current platform for run suggestion
CURRENT_OS=$(uname -s | tr '[:upper:]' '[:lower:]')
CURRENT_ARCH=$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')
echo -e "${YELLOW}💡 To run on this system: ./${BUILD_DIR}/${CURRENT_OS}-${CURRENT_ARCH}/lambda-nat-proxy --help${NC}"