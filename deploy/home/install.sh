#!/bin/bash
set -e

echo "Installing Hookly Home Hub..."

# Check for required files
if [ ! -f ".env" ]; then
    echo "Error: .env file not found. Copy .env.example to .env and configure it."
    exit 1
fi

# Create directory
sudo mkdir -p /opt/hookly-home

# Copy files
sudo cp docker-compose.yml /opt/hookly-home/
sudo cp .env /opt/hookly-home/

# Set permissions
sudo chmod 600 /opt/hookly-home/.env

# Install systemd unit
sudo cp hookly-home.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable hookly-home

echo "Installation complete!"
echo ""
echo "To start the service:"
echo "  sudo systemctl start hookly-home"
echo ""
echo "To view logs:"
echo "  sudo journalctl -u hookly-home -f"
