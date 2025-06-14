#!/bin/bash

# Build Dashboard Script
# This script builds the React frontend and prepares it for embedding

set -e

echo "🔨 Building React frontend..."
cd web
npm run build

echo "📁 Copying build files to Go embed location..."
cd ..
rm -rf internal/dashboard/web/dist
mkdir -p internal/dashboard/web
cp -r web/dist internal/dashboard/web/

echo "✅ Dashboard frontend build complete!"
echo "💡 Dashboard is now ready for embedding in the Go binary"