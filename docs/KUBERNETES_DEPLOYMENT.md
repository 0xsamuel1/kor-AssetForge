# Kubernetes deployment

The repository includes a standalone manifest at `kubernetes/deployment.yaml`
and a configurable Helm chart at `helm/kor-assetforge`.

Create the secret before deploying:

```sh
kubectl create secret generic assetforge-api-secrets -n assetforge \
  --from-literal=DATABASE_URL='postgres://...' \
  --from-literal=REDIS_URL='redis://...' \
  --from-literal=JWT_SECRET='...'
```

For a direct deployment, apply the manifest:

```sh
kubectl apply -f kubernetes/deployment.yaml
```

For Helm, set the image and ingress values for the target cluster:

```sh
helm upgrade --install assetforge helm/kor-assetforge \
  --namespace assetforge --create-namespace
```

The deployment defines requests and limits, health probes, an ingress, and a
CPU/memory-backed HPA. The production workflow is manual and requires the
`KUBE_CONFIG` environment secret.
