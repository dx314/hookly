#!/bin/bash
set -e

echo "Installing Hookly Edge Gateway..."

# Check for required files
if [ ! -f ".env" ]; then
    echo "Error: .env file not found. Copy .env.example to .env and configure it."
    exit 1
fi

# Create directory
sudo mkdir -p /opt/hookly-edge
sudo mkdir -p /opt/hookly-edge/data

# Copy files
sudo cp docker-compose.yml /opt/hookly-edge/
sudo cp .env /opt/hookly-edge/

# Set permissions
sudo chmod 600 /opt/hookly-edge/.env

# Install systemd unit
sudo cp hookly-edge.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable hookly-edge

echo "Installation complete!"
echo ""
echo "To start the service:"
echo "  sudo systemctl start hookly-edge"
echo ""
echo "To view logs:"
echo "  sudo journalctl -u hookly-edge -f"
