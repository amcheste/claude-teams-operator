#!/bin/bash
set -euo pipefail

# hack/kind-setup.sh — Creates a Kind cluster with NFS for ReadWriteMany PVCs.

CLUSTER_NAME="${KIND_CLUSTER_NAME:-claude-teams}"

echo "=== Claude Teams Operator — Kind Dev Setup ==="
echo ""

# 1. Create Kind cluster
echo "[1/5] Creating Kind cluster: ${CLUSTER_NAME}"
cat <<EOF | kind create cluster --name "${CLUSTER_NAME}" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    kubeadmConfigPatches:
      - |
        kind: InitConfiguration
        nodeRegistration:
          kubeletExtraArgs:
            node-labels: "ingress-ready=true"
  - role: worker
  - role: worker
  - role: worker
EOF

echo ""
echo "[2/5] Installing NFS provisioner for ReadWriteMany PVCs"
helm repo add nfs-ganesha https://kubernetes-sigs.github.io/nfs-ganesha-server-and-external-provisioner/ 2>/dev/null || true
helm repo update
helm install nfs-server nfs-ganesha/nfs-server-provisioner \
  --namespace nfs \
  --create-namespace \
  --set persistence.enabled=true \
  --set persistence.size=50Gi \
  --set storageClass.name=nfs \
  --set storageClass.defaultClass=false \
  --wait \
  --timeout=15m
# 15m is deliberate: the nfs-provisioner image is ~500MB and first-time
# pulls routinely take 5–10 minutes on home connections or behind slow
# registry mirrors. Helm's default --wait timeout of 5m was hitting failures
# here even though the deployment itself was healthy; the resources would
# come up a minute later but with the release marked "failed", confusing
# contributors into thinking the setup was broken.

echo ""
echo "[3/5] Creating dev-agents namespace"
kubectl create namespace dev-agents --dry-run=client -o yaml | kubectl apply -f -

echo ""
echo "[4/5] Creating secrets (placeholder — update with real values)"

# Anthropic API key
if [[ -n "${ANTHROPIC_API_KEY:-}" ]]; then
    kubectl create secret generic anthropic-api-key \
        --namespace dev-agents \
        --from-literal=ANTHROPIC_API_KEY="${ANTHROPIC_API_KEY}" \
        --dry-run=client -o yaml | kubectl apply -f -
    echo "  ✓ anthropic-api-key secret created from env var"
else
    echo "  ⚠ ANTHROPIC_API_KEY not set — create secret manually:"
    echo "    kubectl create secret generic anthropic-api-key \\"
    echo "      --namespace dev-agents \\"
    echo "      --from-literal=ANTHROPIC_API_KEY=sk-ant-..."
fi

# Git credentials
if [[ -f "${HOME}/.ssh/id_ed25519" ]]; then
    kubectl create secret generic git-credentials \
        --namespace dev-agents \
        --from-file=ssh-privatekey="${HOME}/.ssh/id_ed25519" \
        --dry-run=client -o yaml | kubectl apply -f -
    echo "  ✓ git-credentials secret created from ~/.ssh/id_ed25519"
else
    echo "  ⚠ SSH key not found — create secret manually:"
    echo "    kubectl create secret generic git-credentials \\"
    echo "      --namespace dev-agents \\"
    echo "      --from-file=ssh-privatekey=~/.ssh/id_ed25519"
fi

echo ""
echo "[5/5] Cluster ready!"
echo ""
echo "Next steps:"
echo "  1. Build and load images:"
echo "     make docker-build docker-build-runner kind-load"
echo ""
echo "  2. Install CRDs and deploy operator:"
echo "     make install deploy"
echo ""
echo "  3. Deploy a sample team:"
echo "     make sample"
echo ""
kubectl cluster-info --context "kind-${CLUSTER_NAME}"
