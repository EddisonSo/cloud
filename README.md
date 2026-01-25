# Edd-Cloud

Unified monorepo for all Edd-Cloud services.

## Structure

- `go-gfs/` - Distributed file system (shared library)
- `edd-cloud-interface/` - Core services (sfs, logging, compute, frontend)
- `edd-cloud-auth/` - Authentication service
- `edd-gateway/` - API gateway
- `edd-cloud-docs/` - Documentation site
- `cluster-monitor/` - Cluster monitoring
- `log-service/` - Log aggregation service
- `manifests/` - Kubernetes manifests
- `proto/` - Shared protobuf definitions

## CI/CD

The unified workflow at `.github/workflows/build-deploy.yml` automatically:
- Detects which services changed
- Builds only the affected services
- Deploys to Kubernetes

You can also force rebuild all services using workflow_dispatch with `force_all: true`.
