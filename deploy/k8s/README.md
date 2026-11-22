# Kubernetes manifests

Apply-able skeletons for Deployment / Service / HPA / canary. **Not** a production cluster.

| File | Purpose |
| --- | --- |
| `namespace.yaml` | `earnings-sync` namespace |
| `configmap.yaml` | Non-secret env (`STORAGE_MODE`, flags) |
| `deployment.yaml` | Stable Deployment (`slot: blue`) + `/healthz` probes |
| `canary-deployment.yaml` | Canary Deployment (`slot: canary`) |
| `service.yaml` | ClusterIP; flip `slot` for cutover |
| `hpa.yaml` | CPU HPA (needs metrics-server) |
| `kustomization.yaml` | `kubectl apply -k deploy/k8s` |
| `validate.sh` | Local check: kubectl dry-run **or** YAML structural fallback |

## Local validation (no cluster)

```bash
make k8s-validate
# or
./deploy/k8s/validate.sh
```

## Apply (when you have a cluster)

```bash
docker build -t earnings-sync:local .
kubectl apply -k deploy/k8s
kubectl -n earnings-sync get deploy,svc,hpa
```

## Blue / green / canary

1. Deploy blue (`deployment.yaml`).
2. Deploy canary (`canary-deployment.yaml`) with the new image.
3. Drive flow gray release via `/v1/canary/releases` (in-process state machine; works without mesh).
4. When ready, patch Service selector `slot: canary` (or `green`) and/or `POST .../promote`.
5. Keep blue for rollback; `POST .../rollback` freezes the release object.

Sheets + PostgreSQL dual-write is orthogonal: set `STORAGE_MODE` / `DATABASE_URL` on the Pod.
