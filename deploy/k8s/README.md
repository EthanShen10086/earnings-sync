# Kubernetes placeholders

These manifests are **skeletons**, not a production cluster:

| File | Purpose |
| --- | --- |
| `deployment.yaml` | Single Deployment (`slot: blue`) with `/healthz` probes |
| `service.yaml` | ClusterIP; flip `slot` for blue/green cutover comments |
| `hpa.yaml` | CPU HPA stub (needs metrics-server) |

## Blue / green (commented workflow)

1. Deploy `earnings-sync` with `slot: blue` (default).
2. Apply a twin Deployment named `earnings-sync-green` with `slot: green` and the new image.
3. When green is Ready, patch Service selector `slot: green`.
4. Keep blue around for fast rollback; delete after soak.

No Service Mesh / Argo Rollouts wiring in this round.

## Local compose still preferred

For developer machines use `deploy/docker-compose.yml` profiles (`cache`, `mq`, `objects`, `observe`) rather than these manifests.
