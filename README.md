# Edd-Cloud

Unified monorepo for all Edd-Cloud services.

## Structure

Each backend service is an independent Go module with its own Dockerfile, built and
deployed independently by CI.

- `go-gfs/` - Distributed file system (master + chunkservers, plus a consumed Go SDK)
- `compute/` - Compute service (containers, SSH, ingress)
- `sfs/` - Shared file system / storage service
- `registry/` - Container image registry (OCI)
- `pkg/events/` - Shared event/publisher package used by backend services
- `edd-cloud-interface/frontend/` - Dashboard frontend (React)
- `edd-cloud-auth/` - Authentication service
- `edd-gateway/` - API gateway
- `edd-cloud-docs/` - Documentation site
- `cluster-monitor/` - Cluster monitoring (also serves `health.cloud.eddisonso.com`)
- `log-service/` - Log aggregation service
- `notification-service/` - User notification delivery
- `alerting-service/` - Event-driven alerting
- `cluster-manager/` - Node management utilities
- `manifests/` - Kubernetes manifests
- `proto/` - Shared protobuf definitions

## CI/CD

The unified workflow at `.github/workflows/build-deploy.yml` automatically:
- Detects which services changed
- Builds only the affected services
- Deploys to Kubernetes

You can also force rebuild all services using workflow_dispatch with `force_all: true`.
