#!/usr/bin/env bash
set -euxo pipefail

# PostgreSQL installation and configuration script
# This script installs PostgreSQL, configures it for remote access, and creates a database

# Configuration (injected by deployer)
POSTGRES_DB="{{POSTGRES_DB}}"
POSTGRES_USER="{{POSTGRES_USER}}"
POSTGRES_PASSWORD="{{POSTGRES_PASSWORD}}"

# Install PostgreSQL (non-interactive to avoid tzdata prompt)
export DEBIAN_FRONTEND=noninteractive
sudo -E apt-get update
sudo -E apt-get install -y postgresql postgresql-contrib

# Get PostgreSQL version for config path
PG_VERSION=$(psql --version | awk '{print $3}' | cut -d. -f1)
PG_CONF="/etc/postgresql/${PG_VERSION}/main/postgresql.conf"
PG_HBA="/etc/postgresql/${PG_VERSION}/main/pg_hba.conf"

# Configure PostgreSQL to listen on all interfaces
sudo sed -i "s/#listen_addresses = 'localhost'/listen_addresses = '*'/" "$PG_CONF"

# Enable secure password encryption (recommended by Gitea docs)
sudo sed -i "s/#password_encryption = scram-sha-256/password_encryption = scram-sha-256/" "$PG_CONF"

# Configure access settings (following Gitea recommendations)
# Allow local connections for the specific user/database
echo "local   ${POSTGRES_DB}    ${POSTGRES_USER}    scram-sha-256" | sudo tee -a "$PG_HBA"
# Allow remote connections for the specific user/database
echo "host    ${POSTGRES_DB}    ${POSTGRES_USER}    0.0.0.0/0    scram-sha-256" | sudo tee -a "$PG_HBA"

# Start PostgreSQL (may not auto-start during install)
sudo systemctl start postgresql
sudo systemctl enable postgresql

# Wait for PostgreSQL to be ready
sleep 3

# Create database and user following Gitea recommendations:
# - Use CREATE ROLE with LOGIN
# - Create database with proper encoding (template0, UTF8, en_US.UTF-8)
sudo -u postgres psql <<EOF
CREATE ROLE ${POSTGRES_USER} WITH LOGIN PASSWORD '${POSTGRES_PASSWORD}';
CREATE DATABASE ${POSTGRES_DB} WITH OWNER ${POSTGRES_USER} TEMPLATE template0 ENCODING UTF8 LC_COLLATE 'en_US.UTF-8' LC_CTYPE 'en_US.UTF-8';
GRANT ALL PRIVILEGES ON DATABASE ${POSTGRES_DB} TO ${POSTGRES_USER};
EOF

# Restart PostgreSQL to apply config changes
sudo systemctl restart postgresql

# Save credentials to a file for reference
cat <<EOF | sudo tee /home/ubuntu/postgres-credentials.txt
PostgreSQL Credentials
======================
Host: $(hostname -I | awk '{print $1}')
Port: 5432
Database: ${POSTGRES_DB}
Username: ${POSTGRES_USER}
Password: ${POSTGRES_PASSWORD}

Connection string:
postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@$(hostname -I | awk '{print $1}'):5432/${POSTGRES_DB}
EOF

sudo chown ubuntu:ubuntu /home/ubuntu/postgres-credentials.txt
sudo chmod 600 /home/ubuntu/postgres-credentials.txt

echo "PostgreSQL installation complete!"
echo "Credentials saved to /home/ubuntu/postgres-credentials.txt"
