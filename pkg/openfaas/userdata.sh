#!/usr/bin/env bash

#==============================================================================
# OpenFaaS Edge Installation Script
#==============================================================================
# This script installs OpenFaaS Edge, including optional
# components like a private registry and function builder.
#==============================================================================

set -euxo pipefail

#==============================================================================
# CONFIGURATION
#==============================================================================

# Install additional services (registry and function builder)
export INSTALL_REGISTRY=true
export INSTALL_BUILDER=true

#==============================================================================
# SYSTEM PREPARATION and OpenFaaS Edge Installation
#==============================================================================

has_dnf() {
  [ -n "$(command -v dnf)" ]
}

has_apt_get() {
  [ -n "$(command -v apt-get)" ]
}

echo "==> Configuring system packages and dependencies..."

if $(has_apt_get); then
  export HOME=/home/ubuntu

  sudo apt update -y

  # Configure iptables-persistent to avoid interactive prompts
  echo iptables-persistent iptables-persistent/autosave_v4 boolean false | sudo debconf-set-selections
  echo iptables-persistent iptables-persistent/autosave_v6 boolean false | sudo debconf-set-selections

  arkade oci install --path . ghcr.io/openfaasltd/faasd-pro-debian:latest
  sudo apt install ./openfaas-edge-*-amd64.deb --fix-broken -y

  if [ "${INSTALL_REGISTRY}" = "true" ]; then
    sudo apt install apache2-utils -y
  fi
elif $(has_dnf); then
  export HOME=/home/rocky

  arkade oci install --path . ghcr.io/openfaasltd/faasd-pro-rpm:latest
  sudo dnf install openfaas-edge-*.rpm -y


  if [ "${INSTALL_REGISTRY}" = "true" ]; then
    sudo dnf install httpd-tools -y
  fi
else
    fatal "Could not find apt-get or dnf. Cannot install dependencies on this OS."
    exit 1
fi

# Install faas-cli
arkade get faas-cli --progress=false --path=/usr/local/bin/

# Create the secrets directory and touch the license file
sudo mkdir -p /var/lib/faasd/secrets
touch /var/lib/faasd/secrets/openfaas_license

#==============================================================================
# PRIVATE REGISTRY AND FUNCTION BUILDER SETUP
#==============================================================================

# Always install registry if builder is installed
if [ "${INSTALL_BUILDER}" = "true" ]; then
 INSTALL_REGISTRY=true
fi

if [ "${INSTALL_REGISTRY}" = "true" ]; then
    echo "==> Setting up private container registry..."

    # Generate registry authentication
    export PASSWORD=$(openssl rand -base64 16)
    echo $PASSWORD > $HOME/registry-password.txt

    # Create htpasswd file for registry authentication
    htpasswd -Bbc $HOME/htpasswd faasd $PASSWORD
    sudo mkdir -p /var/lib/faasd/registry/auth
    sudo mv $HOME/htpasswd /var/lib/faasd/registry/auth/htpasswd

    # Create registry configuration
    sudo tee /var/lib/faasd/registry/config.yml > /dev/null <<EOF
version: 0.1
log:
  accesslog:
    disabled: true
  level: warn
  formatter: text

storage:
  filesystem:
    rootdirectory: /var/lib/registry

auth:
  htpasswd:
    realm: basic-realm
    path: /etc/registry/htpasswd

http:
  addr: 0.0.0.0:5000
  relativeurls: false
  draintimeout: 60s
EOF

    # Configure registry authentication for faas-cli
    cat $HOME/registry-password.txt | faas-cli registry-login \
      --server http://registry:5000 \
      --username faasd \
      --password-stdin

    # Setup Docker credentials for faasd-provider
    sudo mkdir -p /var/lib/faasd/.docker
    sudo cp ./credentials/config.json /var/lib/faasd/.docker/config.json

    # Ensure pro-builder can access Docker credentials
    sudo mkdir -p /var/lib/faasd/secrets
    sudo cp ./credentials/config.json /var/lib/faasd/secrets/docker-config

    # Configure local registry hostname resolution
    echo "127.0.0.1 registry" | sudo tee -a /etc/hosts

    echo "==> Adding registry services to docker-compose..."

    # Append additional services to docker-compose.yaml
    sudo tee -a /var/lib/faasd/docker-compose.yaml > /dev/null <<EOF

  registry:
    image: docker.io/library/registry:3
    volumes:
    - type: bind
      source: ./registry/data
      target: /var/lib/registry
    - type: bind
      source: ./registry/auth
      target: /etc/registry/
      read_only: true
    - type: bind
      source: ./registry/config.yml
      target: /etc/docker/registry/config.yml
      read_only: true
    deploy:
      replicas: 1
    ports:
      - "5000:5000"
EOF

fi

if [ "${INSTALL_BUILDER}" = "true" ]; then
    echo "==> Configuring function builder..."

    # Generate payload secret for function builder
    openssl rand -base64 32 | sudo tee /var/lib/faasd/secrets/payload-secret

    echo "==> Adding function builder services to docker-compose..."

    # Append additional services to docker-compose.yaml
    sudo tee -a /var/lib/faasd/docker-compose.yaml > /dev/null <<EOF

  pro-builder:
    depends_on: [buildkit]
    user: "app"
    group_add: ["1000"]
    restart: always
    image: ghcr.io/openfaasltd/pro-builder:0.5.3
    environment:
      buildkit-workspace: /tmp/
      enable_lchown: false
      insecure: true
      buildkit_url: unix:///home/app/.local/run/buildkit/buildkitd.sock
      disable_hmac: false
      # max_inflight: 10 # Uncomment to limit concurrent builds
    command:
     - "./pro-builder"
     - "-license-file=/run/secrets/openfaas-license"
    volumes:
      - type: bind
        source: ./secrets/payload-secret
        target: /var/openfaas/secrets/payload-secret
      - type: bind
        source: ./secrets/openfaas_license
        target: /run/secrets/openfaas-license
      - type: bind
        source: ./secrets/docker-config
        target: /home/app/.docker/config.json
      - type: bind
        source: ./buildkit-rootless-run
        target: /home/app/.local/run
      - type: bind
        source: ./buildkit-sock
        target: /home/app/.local/run/buildkit
    deploy:
      replicas: 1
    ports:
     - "8088:8080"

  buildkit:
    restart: always
    image: docker.io/moby/buildkit:v0.23.2-rootless
    group_add: ["2000"]
    user: "1000:1000"
    cap_add:
      - CAP_SETUID
      - CAP_SETGID
    command:
    - rootlesskit
    - buildkitd
    - "--addr"
    - unix:///home/user/.local/share/bksock/buildkitd.sock
    - --oci-worker-no-process-sandbox
    security_opt:
    - no-new-privileges=false
    - seccomp=unconfined        # Required for mount(2) syscall
    volumes:
      # Runtime directory for rootlesskit/buildkit socket
      - ./buildkit-rootless-run:/home/user/.local/run
      - /sys/fs/cgroup:/sys/fs/cgroup
      # Persistent state and cache directories
      - ./buildkit-rootless-state:/home/user/.local/share/buildkit
      - ./buildkit-sock:/home/user/.local/share/bksock
    environment:
      XDG_RUNTIME_DIR: /home/user/.local/run
      TZ: "UTC"
      BUILDKIT_DEBUG: "1"         # Enable for debugging
      BUILDKIT_EXPERIMENTAL: "1"  # Enable experimental features
    deploy:
      replicas: 1
EOF

fi

#==============================================================================
# INSTALLATION EXECUTION
#==============================================================================

echo "==> Installing faasd..."

# Execute the installation
sudo /usr/local/bin/faasd install

#==============================================================================
# POST-INSTALLATION CONFIGURATION
#==============================================================================

if [ "${INSTALL_BUILDER}" = "true" ]; then
    echo "==> Configuring insecure registry access..."

    # Configure faasd-provider to use insecure registry
    sudo sed -i '/^ExecStart=/ s|$| --insecure-registry http://registry:5000|' \
        /lib/systemd/system/faasd-provider.service

    # Reload systemd and restart faasd-provider
    sudo systemctl daemon-reload
    sudo systemctl restart faasd-provider
fi

echo "==> OpenFaaS Edge installation completed successfully!"
echo ""
echo "1. Access the OpenFaaS gateway at http://localhost:8080"
echo "2. Get your admin password: sudo cat /var/lib/faasd/secrets/basic-auth-password"
if [ "${INSTALL_REGISTRY}" = "true" ]; then
    echo "3. Private registry available at http://localhost:5000"
    echo "4. Registry password: cat $HOME/registry-password.txt"
fi
if [ "${INSTALL_BUILDER}" = "true" ]; then
    echo "5. Pro-builder service available at http://localhost:8088"
fi
