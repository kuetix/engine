#!/bin/bash
set -e

# Build script for creating Debian package

VERSION=${VERSION:-0.1.0}
ARCH=${ARCH:-amd64}
BUILD_DIR="build/deb"
PACKAGE_NAME="kue_${VERSION}_${ARCH}"

echo "Building Debian package for kue version ${VERSION}..."

# Clean previous builds
rm -rf ${BUILD_DIR}
mkdir -p ${BUILD_DIR}/${PACKAGE_NAME}/DEBIAN
mkdir -p ${BUILD_DIR}/${PACKAGE_NAME}/usr/bin

# Build the binary
echo "Building kue binary..."
go build -ldflags="-s -w -X main.Version=${VERSION}" -o ${BUILD_DIR}/${PACKAGE_NAME}/usr/bin/kue ./cmd/kue

# Copy control file
cp packaging/debian/control ${BUILD_DIR}/${PACKAGE_NAME}/DEBIAN/control

# Update version in control file
sed -i "s/Version: .*/Version: ${VERSION}/" ${BUILD_DIR}/${PACKAGE_NAME}/DEBIAN/control
sed -i "s/Architecture: .*/Architecture: ${ARCH}/" ${BUILD_DIR}/${PACKAGE_NAME}/DEBIAN/control

# Build the package
echo "Creating .deb package..."
dpkg-deb --build ${BUILD_DIR}/${PACKAGE_NAME}

# Move to output directory
mkdir -p dist
mv ${BUILD_DIR}/${PACKAGE_NAME}.deb dist/

echo "Package created: dist/${PACKAGE_NAME}.deb"
echo "Done!"
