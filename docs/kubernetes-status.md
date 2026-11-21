# Kubernetes status

Kubernetes is intentionally deferred. This phase completes and verifies the
Docker Compose platform, CI/CD, security hardening and the M1 documentation.

## Current state

- No Kubernetes manifests, Helm chart or Kustomize overlay exists in the
  repository.
- The supported local and CI deployment is `docker-compose.yml`.
- Images are built for `linux/amd64` and published to GHCR by the production
  workflow.
- No cluster, ingress controller, cert-manager or production TLS is provisioned.

## Rationale

The M1 acceptance criteria are satisfied by a verified Compose stack. Adding
unverified manifests would not prove readiness and would compete for time with
the tested deployment path. The deferred work is recorded here rather than being
described as implemented.

## Deferred work

When Kubernetes is scheduled, the minimum deliverable is:

- ConfigMaps for non-secret configuration and Secrets for credentials.
- Deployments and Services for identity, parcel, pricing, merchant, realtime,
  ai-assistant, notification-worker, webhook-worker, web and Nginx (or an
  ingress replacement).
- Readiness and liveness probes matching `/ready` and `/health`.
- Resource requests and limits.
- Persistent storage for PostgreSQL and proof objects, or managed alternatives.
- Ingress with TLS termination.
- Migration execution as a Job, not at application startup.
- Shared state for realtime fan-out and AI rate/conversation limits before
  scaling beyond one replica.

Until then, do not claim Kubernetes support.
