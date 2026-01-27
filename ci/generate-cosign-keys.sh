#!/usr/bin/env bash
# Script to generate cosign key pair for image signing

set -euo pipefail

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Generating Cosign Key Pair${NC}"
echo ""

# Check if cosign is installed
if ! command -v cosign &> /dev/null; then
    echo -e "${YELLOW}cosign is not installed. Installing...${NC}"

    # Detect OS
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    if [ "$ARCH" = "x86_64" ]; then
        ARCH="amd64"
    elif [ "$ARCH" = "aarch64" ]; then
        ARCH="arm64"
    fi

    # Download cosign
    COSIGN_VERSION="v2.2.2"
    curl -sLO "https://github.com/sigstore/cosign/releases/download/${COSIGN_VERSION}/cosign-${OS}-${ARCH}"
    chmod +x "cosign-${OS}-${ARCH}"
    sudo mv "cosign-${OS}-${ARCH}" /usr/local/bin/cosign

    echo -e "${GREEN}cosign installed${NC}"
fi

# Generate key pair
echo ""
echo "Generating key pair..."
echo "You will be prompted to enter a password to encrypt the private key."
echo ""

cosign generate-key-pair

echo ""
echo -e "${GREEN}Key pair generated successfully!${NC}"
echo ""
echo "Files created:"
echo "  - cosign.key (private key - keep this secret!)"
echo "  - cosign.pub (public key - can be shared)"
echo ""
echo "Next steps:"
echo "1. Add the contents of cosign.key to ci/credentials.yml as cosign_private_key"
echo "2. Add the contents of cosign.pub to ci/credentials.yml as cosign_public_key"
echo "3. Add the password you entered to ci/credentials.yml as cosign_password"
echo ""
echo -e "${YELLOW}IMPORTANT: Never commit cosign.key or credentials.yml to git!${NC}"
