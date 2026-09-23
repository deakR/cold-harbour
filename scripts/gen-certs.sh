#!/bin/sh
set -eu
out="${1:-docker/certs}"
mkdir -p "$out"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

openssl req -x509 -newkey rsa:2048 -nodes -keyout "$tmp/ca.key" -out "$out/ca.crt" -days 825 -subj "/CN=coldharbour-ca"

cat > "$tmp/server.cnf" <<'EOF'
[req]
distinguished_name = dn
req_extensions = ext
prompt = no
[dn]
CN = coldharbour
[ext]
subjectAltName = DNS:postgres,DNS:redis,DNS:localhost,IP:127.0.0.1
extendedKeyUsage = serverAuth
basicConstraints = CA:FALSE
keyUsage = digitalSignature,keyEncipherment
EOF

openssl req -newkey rsa:2048 -nodes -keyout "$out/server.key" -out "$tmp/server.csr" -config "$tmp/server.cnf"
openssl x509 -req -in "$tmp/server.csr" -CA "$out/ca.crt" -CAkey "$tmp/ca.key" -CAcreateserial -out "$out/server.crt" -days 825 -extfile "$tmp/server.cnf" -extensions ext
chmod 644 "$out/server.crt" "$out/ca.crt"
chmod 600 "$out/server.key"
