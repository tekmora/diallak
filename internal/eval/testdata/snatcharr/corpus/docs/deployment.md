<!--
SPDX-FileCopyrightText: 2026 lusoris <lusoris@pm.me>
SPDX-License-Identifier: CC-BY-SA-4.0
-->

# Deployment

Two images, one chart. The API image carries the UI; the worker only dials the API.
Both are distroless static, non-root (65532), read-only, and satisfy the *restricted*
Pod Security Standard.

| Artifact | Where |
| --- | --- |
| `ghcr.io/lusoris/snatcharr-api` | Go control plane + embedded SPA (HTTP 8080, gRPC 9090) |
| `ghcr.io/lusoris/snatcharr-worker` | Rust snatch-worker (no listener) |
| `oci://ghcr.io/lusoris/charts/snatcharr` | Helm chart |

Every image and the chart are signed with cosign (keyless, GitHub OIDC) and ship an SPDX
SBOM and SLSA provenance attestation. Tags are semver only: there is no `latest`.

```sh
cosign verify ghcr.io/lusoris/snatcharr-api:0.1.0 \
  --certificate-identity-regexp 'https://github.com/lusoris/SnatchArr/' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

## Helm

Requirements: Kubernetes 1.29+, and the
[CloudNativePG](https://cloudnative-pg.io/) operator unless you bring your own Postgres.

```sh
helm install snatcharr oci://ghcr.io/lusoris/charts/snatcharr \
  --namespace snatcharr --create-namespace \
  --set config.publicUrl=https://snatcharr.example.com \
  --set api.ingress.enabled=true \
  --set 'api.ingress.hosts[0].host=snatcharr.example.com' \
  --set 'api.ingress.hosts[0].paths[0].path=/' \
  --set 'api.ingress.hosts[0].paths[0].pathType=Prefix' \
  --set networkPolicy.enabled=true
```

The first visit runs the setup wizard. `helm status` prints the port-forward command when
no Ingress or HTTPRoute is enabled.

### What the chart renders

| Resource | Notes |
| --- | --- |
| Deployment `-api` | probes on `/startupz`, `/livez`, `/readyz`; `/tmp` is a memory-backed emptyDir; `replicaCount` may exceed 1, periodic work runs on the elected replica |
| Deployment `-worker` | dials `<release>-api:9090`; optional HPA 1..4 on CPU |
| Service `-api` | `http` 8080, `grpc` 9090 (h2c) |
| Ingress or HTTPRoute | `api.ingress.*` or `api.httpRoute.*` |
| CNPG `Cluster` `-db` | `postgres.cnpg.*`; the API reads the DSN from the operator's `-db-app` Secret |
| Secret `-secrets` | crypto key, CSRF secret, API-key HMAC secret, worker token |
| NetworkPolicy | worker may only reach the API, `networkPolicy.arrCidrs` and `networkPolicy.arrPeers` (both empty allow no *arr egress, never all of it); gRPC only from the worker |
| ServiceMonitor | `serviceMonitor.enabled` scrapes `/metrics` on the API |
| PodDisruptionBudget | per component, opt-in |

### NetworkPolicy and in-cluster *arr apps

`networkPolicy.arrCidrs` allows egress by IP range. That reaches *arr apps outside the
cluster, but some CNIs never apply CIDR rules to traffic between pods they manage: Cilium
states that "CIDR rules do not apply to traffic where both sides of the connection are
either managed by Cilium" (its layer 3 policy docs). If your *arr apps, Seerr, Cleanuparr
or download clients run as pods in the same cluster, select them in `networkPolicy.arrPeers`:

```yaml
networkPolicy:
  enabled: true
  arrPeers:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: media
```

`arrPeers` accepts `podSelector` and `namespaceSelector` only; IP ranges belong in
`arrCidrs`, which always excludes the cloud metadata address.

### Secrets

The chart needs four secrets. Leave them empty and Helm generates them once, reusing the
existing Secret on upgrades (`helm.sh/resource-policy: keep`). Tools that render without
cluster access, such as ArgoCD, cannot reuse them, so create the Secret first:

```sh
kubectl -n snatcharr create secret generic snatcharr-secrets \
  --from-literal=crypto-key="$(openssl rand -hex 32)" \
  --from-literal=csrf-secret="$(openssl rand -hex 32)" \
  --from-literal=apikey-secret="$(openssl rand -base64 36)" \
  --from-literal=worker-token="$(openssl rand -base64 36)"
```

and set `secrets.existingSecret=snatcharr-secrets`. The crypto key encrypts every *arr
API key and download-client password at rest; losing it means re-entering them.

### Postgres

`postgres.cnpg.enabled=true` (default) renders a CloudNativePG `Cluster` with
`postgres.cnpg.instances` replicas and `postgres.cnpg.storage.size` per instance;
`postgres.cnpg.extraSpec` is merged verbatim for backups, affinity or parameters.
For an external database set `postgres.cnpg.enabled=false` and point
`postgres.external.dsnSecretRef` at a Secret holding the DSN. Only the API connects to
Postgres.

`sslmode=require` in the DSN encrypts the connection without checking the server
certificate. To verify it against the cluster CA (`verify-full`), mount the CA and point
the API at it. The settings override the DSN, and the CA file is re-read for every new
connection, so a rotated CA applies without a restart. With CloudNativePG the CA lives in
the `<cluster>-ca` Secret; mount only its `ca.crt`, never `ca.key`:

```yaml
config:
  extraEnv:
    - name: APP_DB_SSL_MODE
      value: verify-full
    - name: APP_DB_SSL_ROOTCERT
      value: /etc/snatcharr/db-ca/ca.crt
api:
  extraVolumes:
    - name: db-ca
      secret:
        secretName: pg-cluster-ca
        items:
          - key: ca.crt
            path: ca.crt
  extraVolumeMounts:
    - name: db-ca
      mountPath: /etc/snatcharr/db-ca
      readOnly: true
```

`verify-full` also checks the host name, so the DSN must name a host on the server
certificate, such as CloudNativePG's `<cluster>-rw.<namespace>.svc`.

The schema migrations that run at start use the same settings: SnatchArr adds the
`APP_DB_SSL_*` values (and, when set, the password from the `db.password_file` config key)
to the DSN it hands the migrator. Before 0.2 the migrator ignored them and the API crash-looped on
`x509: certificate signed by unknown authority`; on 0.1.x, add `sslrootcert=<path>` to the
DSN itself. An explicit `APP_DB_MIGRATE_DSN` is used exactly as given.

### Configarr

Already running [Configarr](https://github.com/raydak-labs/configarr)? Mount the same
files and SnatchArr imports every instance from them (linked mode):

```yaml
configarr:
  enabled: true
  existingConfigMap: configarr-config   # key config.yml
  existingSecret: configarr-secrets     # key secrets.yml (optional)
```

## ArgoCD

`deploy/argocd/` holds an `AppProject` and an `Application` that track `main` with
automated prune and self-heal, server-side apply and retries. Override values under
`spec.source.helm.valuesObject`; keep `secrets.existingSecret` set, see above.

## Local cluster

`make kind-up` builds both images, starts an incus VM (`tools/incus-kind-vm.sh`, 6 CPUs
and 12 GiB by default), creates a kind cluster inside it, installs CloudNativePG and
applies `deploy/kustomize/kind` (restricted namespace, NetworkPolicy on, throwaway
secrets). kind never runs in host Docker: its kubelet changes kernel settings such as
`vm.overcommit_memory` that are not namespaced, so on the host it would change the
workstation's kernel (#61).

The host reaches the cluster through a dedicated kubeconfig:

```bash
export KUBECONFIG=$PWD/.kind/snatcharr.kubeconfig
kubectl -n snatcharr port-forward svc/snatcharr-api 8080:8080
```

`make kind-down` deletes the VM, its profile and the kubeconfig. The VM needs incus with
KVM on the host; `SNATCHARR_KIND_VM_CPUS` and `SNATCHARR_KIND_VM_MEMORY` size it. For plain
local development `make dev` starts Postgres with Docker Compose and runs the three
processes on the host.
