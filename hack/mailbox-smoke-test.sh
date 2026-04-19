#!/usr/bin/env bash
# hack/mailbox-smoke-test.sh
#
# Proves the core architectural claim of the operator: two pods sharing a
# ReadWriteMany PVC can exchange JSON mailbox files as Claude Code's native
# Agent Teams protocol expects.
#
# The script creates a PVC that matches the operator's team-state PVC
# (same StorageClass, size, and mount path) and spawns two long-lived busybox
# pods in the role of lead + teammate. It then:
#   1. writes an inbox JSON from the lead pod
#   2. reads it back from the teammate pod
#   3. writes a reply from the teammate pod
#   4. reads the reply back from the lead pod
#
# Bypassing the AgentTeam CRD here is deliberate: the acceptance-mode operator
# spawns pods with a 20-second lifetime which does not leave room for reliable
# kubectl-exec round-trips. This script isolates the PVC + mount behaviour that
# is the real subject of the test. The operator's own PVC creation is already
# covered by the Ginkgo acceptance suite (waitForPVC assertions).
#
# Usage:
#   make acceptance-up          # or any Kind cluster with an 'nfs' StorageClass
#   bash hack/mailbox-smoke-test.sh
#
# Requires: kubectl, a reachable Kubernetes cluster, the 'nfs' StorageClass
# alias that the operator expects (created by hack/acceptance-setup.sh).

set -euo pipefail

# Extend PATH to pick up Homebrew / Docker Desktop binaries when run outside
# an interactive shell.
export PATH="/opt/homebrew/bin:/usr/local/bin:${PATH}"

CLUSTER_NAME="${KIND_CLUSTER_NAME:-claude-teams}"
TEST_NS="mailbox-smoke-$(date +%s)"
TEAM_NAME="mailbox-demo"
PVC_NAME="${TEAM_NAME}-team-state"
PVC_SIZE="${PVC_SIZE:-1Gi}"
STORAGE_CLASS="${STORAGE_CLASS:-nfs}"
# Kind's local-path provisioner only supports RWO. The script still proves
# the architectural claim on single-node clusters because hostPath volumes are
# visible to every pod on that node. Override with RWM on a real cluster.
PVC_ACCESS_MODE="${PVC_ACCESS_MODE:-ReadWriteOnce}"
MOUNT_PATH="/var/claude-state"
READY_TIMEOUT_SEC="${READY_TIMEOUT_SEC:-120}"

log() { echo "==> $*"; }
fail() { echo "[FAIL] $*" >&2; exit 1; }

cleanup() {
  local rc=$?
  log "Cleaning up namespace ${TEST_NS}"
  kubectl delete namespace "${TEST_NS}" --wait=false --ignore-not-found >/dev/null 2>&1 || true
  exit "${rc}"
}
trap cleanup EXIT

# ── Preflight ──────────────────────────────────────────────────────────────
log "Verifying cluster connectivity"
if ! kubectl cluster-info >/dev/null 2>&1; then
  fail "No reachable cluster. Run 'make acceptance-up' first."
fi

log "Verifying '${STORAGE_CLASS}' StorageClass exists"
if ! kubectl get storageclass "${STORAGE_CLASS}" >/dev/null 2>&1; then
  fail "StorageClass '${STORAGE_CLASS}' not found. Run 'make acceptance-up' first or set STORAGE_CLASS=<name>."
fi

# ── Create test namespace and PVC ──────────────────────────────────────────
log "Creating namespace ${TEST_NS}"
kubectl create namespace "${TEST_NS}" >/dev/null

log "Creating team-state PVC ${PVC_NAME} (${PVC_ACCESS_MODE}, ${PVC_SIZE}, storageClass=${STORAGE_CLASS})"
kubectl apply -n "${TEST_NS}" -f - <<YAML >/dev/null
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: ${PVC_NAME}
spec:
  accessModes: ["${PVC_ACCESS_MODE}"]
  storageClassName: ${STORAGE_CLASS}
  resources:
    requests:
      storage: ${PVC_SIZE}
YAML

# ── Deploy lead + teammate pods ────────────────────────────────────────────
deploy_pod() {
  local name="$1"
  kubectl apply -n "${TEST_NS}" -f - <<YAML >/dev/null
apiVersion: v1
kind: Pod
metadata:
  name: ${name}
  labels:
    app: mailbox-smoke
    role: ${name}
spec:
  restartPolicy: Never
  containers:
    - name: agent
      image: busybox:latest
      command: ["sh", "-c", "sleep 600"]
      volumeMounts:
        - name: team-state
          mountPath: ${MOUNT_PATH}
  volumes:
    - name: team-state
      persistentVolumeClaim:
        claimName: ${PVC_NAME}
YAML
}

log "Deploying lead pod"
deploy_pod "${TEAM_NAME}-lead"
log "Deploying teammate pod"
deploy_pod "${TEAM_NAME}-teammate"

wait_for_ready() {
  local pod="$1"
  log "Waiting up to ${READY_TIMEOUT_SEC}s for pod ${pod} to be Ready"
  if ! kubectl wait -n "${TEST_NS}" --for=condition=Ready "pod/${pod}" --timeout="${READY_TIMEOUT_SEC}s" >/dev/null; then
    kubectl -n "${TEST_NS}" describe "pod/${pod}" >&2 || true
    fail "pod ${pod} did not become Ready within ${READY_TIMEOUT_SEC}s"
  fi
}

wait_for_ready "${TEAM_NAME}-lead"
wait_for_ready "${TEAM_NAME}-teammate"

# ── Exchange mailbox files ─────────────────────────────────────────────────
INBOX_DIR="${MOUNT_PATH}/teams/${TEAM_NAME}/inboxes"
LEAD_MESSAGE='{"from":"lead","to":"teammate","subject":"ping","body":"mailbox exchange verified lead->teammate"}'
TEAMMATE_REPLY='{"from":"teammate","to":"lead","subject":"pong","body":"mailbox exchange verified teammate->lead"}'

log "Writing inbox JSON from lead pod"
kubectl -n "${TEST_NS}" exec "${TEAM_NAME}-lead" -- sh -c \
  "mkdir -p ${INBOX_DIR} && printf '%s' '${LEAD_MESSAGE}' > ${INBOX_DIR}/teammate.json"

log "Reading inbox JSON from teammate pod"
READ_BY_TEAMMATE="$(kubectl -n "${TEST_NS}" exec "${TEAM_NAME}-teammate" -- cat "${INBOX_DIR}/teammate.json")"

if [[ "${READ_BY_TEAMMATE}" != "${LEAD_MESSAGE}" ]]; then
  echo "expected: ${LEAD_MESSAGE}" >&2
  echo "got:      ${READ_BY_TEAMMATE}" >&2
  fail "teammate pod could not read the message written by lead"
fi
log "  ✓ teammate sees lead's message"

log "Writing reply from teammate pod"
kubectl -n "${TEST_NS}" exec "${TEAM_NAME}-teammate" -- sh -c \
  "mkdir -p ${INBOX_DIR} && printf '%s' '${TEAMMATE_REPLY}' > ${INBOX_DIR}/lead.json"

log "Reading reply from lead pod"
READ_BY_LEAD="$(kubectl -n "${TEST_NS}" exec "${TEAM_NAME}-lead" -- cat "${INBOX_DIR}/lead.json")"

if [[ "${READ_BY_LEAD}" != "${TEAMMATE_REPLY}" ]]; then
  echo "expected: ${TEAMMATE_REPLY}" >&2
  echo "got:      ${READ_BY_LEAD}" >&2
  fail "lead pod could not read the reply written by teammate"
fi
log "  ✓ lead sees teammate's reply"

# ── Verify directory listing from each side ────────────────────────────────
log "Verifying both pods see the complete inbox directory"
LEAD_LISTING="$(kubectl -n "${TEST_NS}" exec "${TEAM_NAME}-lead" -- ls "${INBOX_DIR}" | sort | tr '\n' ' ')"
TEAMMATE_LISTING="$(kubectl -n "${TEST_NS}" exec "${TEAM_NAME}-teammate" -- ls "${INBOX_DIR}" | sort | tr '\n' ' ')"

if [[ "${LEAD_LISTING}" != "${TEAMMATE_LISTING}" ]]; then
  echo "lead     sees: ${LEAD_LISTING}" >&2
  echo "teammate sees: ${TEAMMATE_LISTING}" >&2
  fail "pods disagree on inbox directory contents"
fi

if [[ "${LEAD_LISTING}" != "lead.json teammate.json " ]]; then
  fail "unexpected inbox listing (got: '${LEAD_LISTING}')"
fi
log "  ✓ both pods see: ${LEAD_LISTING}"

echo ""
log "PASS — mailbox file exchange verified on shared PVC"
log "      StorageClass=${STORAGE_CLASS} AccessMode=${PVC_ACCESS_MODE} Mount=${MOUNT_PATH}"
