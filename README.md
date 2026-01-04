# OpenStack Config Controller (minimal)

This repository contains a minimal Kubernetes controller that authenticates to OpenStack using `gophercloud` and populates a Custom Resource's status with block storage volumes.

Quick start (local):

1. Set OpenStack env vars (example):

```bash
export OS_AUTH_URL=https://openstack.ayr.ca:5000/v3
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_PROJECT_NAME=demo
export OS_USER_DOMAIN_NAME=default
export OS_PROJECT_DOMAIN_NAME=default
```

2. Build & run locally (uses your kubeconfig to watch CRs):

```bash
go mod download
go run ./... --reconcile-interval=5m
```

3. Apply the CRD and sample CR in your cluster:

```bash
kubectl apply -f config/crd/bases/openstack.ayr.ca_openstackblockstorages.yaml
kubectl apply -f config/samples/openstack_v1alpha1_openstackblockstorage.yaml
```

Notes on reconcile interval:
- CLI flag: `--reconcile-interval` (Go duration string, e.g. `30s`, `5m`)
- Env var override: `OPENSTACK_RECONCILE_INTERVAL` (same format)

Running tests
-
To run the unit tests for the controller and helpers run:

```bash
go test ./...
```

For verbose output or to run a specific package use the standard `go test` flags.

4. Check status on the sample CR:

```bash
kubectl get openstackblockstorage example-blockstorage -o yaml
```

Notes:
- The controller authenticates to OpenStack using standard `OS_*` environment variables.
- The CRD is minimal and intended as a starting point. For production use, add RBAC manifests, leader election, and proper packaging.
