#!/bin/bash

echo "Building dotfiles manager..."
echo "Checking dependencies..."
go mod tidy
echo "Compiling..."
go build -o ../hideDot
chmod +x hideDot
echo "Build complete! You can now run ./hideDot"
