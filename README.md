# OpenStack Config Controller (minimal)

This repository contains a minimal Kubernetes controller that authenticates to OpenStack using `gophercloud` and populates a Custom Resource's status with block storage volumes.

Quick start (local):

1. Set OpenStack env vars (example):

```bash
export OS_AUTH_URL=https://openstack.example.com:5000/v3
export OS_USERNAME=admin
export OS_PASSWORD=secret
export OS_PROJECT_NAME=demo
export OS_USER_DOMAIN_NAME=Default
export OS_PROJECT_DOMAIN_NAME=Default
```

2. Build & run locally (uses your kubeconfig to watch CRs):

```bash
go mod download
go run ./...
```

3. Apply the CRD and sample CR in your cluster:

```bash
kubectl apply -f config/crd/bases/openstack.example.com_openstackblockstorages.yaml
kubectl apply -f config/samples/openstack_v1alpha1_openstackblockstorage.yaml
```

4. Check status on the sample CR:

```bash
kubectl get openstackblockstorage example-blockstorage -o yaml
```

Notes:
- The controller authenticates to OpenStack using standard `OS_*` environment variables.
- The CRD is minimal and intended as a starting point. For production use, add RBAC manifests, leader election, and proper packaging.
