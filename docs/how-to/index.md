# How-to guides

Recipes for solving specific operational tasks. These assume you already have kagents installed and at least a basic working AgentTeam — if not, start with the [Getting Started tutorial](../tutorials/getting-started.md).

## Install

Cloud-specific install paths covering the ReadWriteMany storage configuration that's the actual deployment friction point on each cloud:

- **[Install on Amazon EKS](install/eks.md)** — EFS CSI driver + EFS file system + Access Points
- **[Install on Google GKE](install/gke.md)** — Filestore CSI driver + Filestore instance
- **[Install on Azure AKS](install/aks.md)** — Azure Files CSI driver + Premium NFS share

Each guide ends with the same `make mailbox-smoke-test` verification step.

## Operate

Day-to-day operational tasks once kagents is running:

- **[Expose the dashboard](operate/expose-dashboard.md)** — port-forward for dev, Ingress with basic auth for prod, oauth2-proxy for corporate SSO, namespace-scoping
- **[Configure shared storage](operate/shared-storage.md)** — sizing the team-state / repo / output PVCs, backup strategies per cloud backend, performance tuning recipes
- **[Set budget alerts](operate/budget-alerts.md)** — per-team `budgetLimit`, chart-wide default, webhook events to Slack/PagerDuty, Prometheus alert rules

## Looking for something else?

- **New to the project?** Start with the [Getting Started tutorial](../tutorials/getting-started.md).
- **Want to understand how it works?** See the [Explanation](../explanation/index.md) section.
- **Need a specific CRD field or Helm value?** See the [Reference](../reference/index.md) section.
