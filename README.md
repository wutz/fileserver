# fileserver-operator

A Kubernetes operator that serves the volumes of a namespace over HTTPS with
[dufs](https://github.com/sigoden/dufs), on demand. A `FileServer` runs one
dufs pod while it is in use and scales to zero after an idle timeout, so idle
volumes cost no compute.

- Serves every `ReadWriteMany` PersistentVolumeClaim of a namespace (or a
  listed subset) under `/<claim name>`, read-write, behind basic auth.
- Scales the server to zero after `idleTimeout` without activity. Activity
  is a client heartbeat (the `fileserver.wutz.dev/last-active` annotation) or a
  request seen in the dufs access log.
- Publishes each server at `https://<prefix><namespace>.<domain>` through an
  Ingress, with large uploads allowed and streamed.
- Skips volumes in the trash (the `forgeai.wutz.dev/trash=true` label).

## Install

The operator is published as a Helm chart and a container image:

```bash
helm install fileserver-operator oci://ghcr.io/wutz/charts/fileserver-operator \
  --version 0.2.0 \
  --namespace fileserver-operator-system --create-namespace \
  --set config.ingress.domain=files.example.com
```

Set `config.ingress.domain` to a wildcard DNS suffix that points at your
ingress controller (for example `*.files.example.com`). Without it the operator
runs, but no file server is published and each one reports the `Failed` phase.

## Configuration

The chart renders the operator configuration to a ConfigMap, which the operator
reads at startup and reloads when it changes.

| Value | Default | Description |
|---|---|---|
| `config.ingress.className` | `nginx` | Ingress class of the file server Ingresses. Empty uses the cluster default. |
| `config.ingress.domain` | `""` | Wildcard DNS suffix, e.g. `files.example.com`. Empty means no file server is published. |
| `config.ingress.port` | `443` | External HTTPS port; 443 is left out of the URLs. |
| `config.ingress.tlsSecret` | `""` | Optional TLS Secret copied into each file server namespace. Empty uses the ingress controller's default certificate. |
| `config.fileServer.image` | `docker.io/sigoden/dufs:v0.46.0` | The dufs image. |
| `config.fileServer.idleTimeout` | `30m` | Scale-to-zero timeout when a `FileServer` does not set its own. |
| `config.fileServer.hostPrefix` | `files-` | Prefix of the file server host name. |
| `image.repository` / `image.tag` | `ghcr.io/wutz/fileserver-operator` / chart `appVersion` | Operator image. |
| `replicas` | `1` | Operator replicas. Use more than one with `leaderElection: true`. |
| `leaderElection` | `false` | Enable leader election. |

See [`charts/fileserver-operator/values.yaml`](charts/fileserver-operator/values.yaml)
for every value.

## The FileServer resource

`FileServer` is in the `fileserver.wutz.dev/v1alpha1` group, short name `ffs`.
A client creates one named `default` in each namespace it serves.

```yaml
apiVersion: fileserver.wutz.dev/v1alpha1
kind: FileServer
metadata:
  name: default
  namespace: my-project
  annotations:
    # Heartbeat: keeps the server running while this time is recent.
    fileserver.wutz.dev/last-active: "2026-10-27T08:00:00Z"
spec:
  # Optional: serve only these claims. Empty serves every live ReadWriteMany claim.
  volumes: []
  # Optional: overrides config.fileServer.idleTimeout.
  idleTimeout: 30m
```

Status reports the `phase` (`Idle`, `Starting`, `Ready` or `Failed`), the public
`url`, the name of the basic auth `secret`, the served `volumes`, the
`lastActiveTime`, and a `Ready` condition whose message explains a `Failed` phase.

Each file server gets these objects in its namespace, named `forgeai-files-<name>`:
a Deployment, a Service, and an Ingress. The basic auth Secret is named
`forgeai-fileserver` and holds a generated password; the username is `forgeai`.
All of them are owned by the `FileServer`, so deleting it removes them.

## Compatibility

The trash label, the Secret and object names, and the `app.kubernetes.io/managed-by=forgeai`
labels are kept from the forgeai platform. From 0.2.0 the API group is
`fileserver.wutz.dev`. The upgrade removes the 0.1.0 CRD `fileservers.forgeai.wutz.dev`,
which deletes the FileServers it held. The Deployment, Service, Ingress and Secret that
those FileServers owned are garbage-collected with them, so they are recreated rather
than updated in place, and the Secret gets a newly generated password. forgeai
recreates the `default` FileServer on the next use of the Files view.

## Development

Requirements: Go (see `go.mod`), Docker, Helm 3, and `make`.

```bash
make test        # generate, format, vet, and run the envtest suite
make lint        # golangci-lint
make manifests   # regenerate the CRD and RBAC in config/ after changing the API markers
make generate    # regenerate DeepCopy methods
make run   # run against the current kubeconfig, reading the config from CONFIG_NAMESPACE
make helm-lint helm-template
```

`make test` downloads the envtest control plane binaries (`setup-envtest`), so it
needs network access the first time. The config namespace the operator reads
from is passed with `--config-namespace` (or the `POD_NAMESPACE` environment
variable inside a cluster).

Releases: pushing a `vX.Y.Z` tag publishes the image
`ghcr.io/wutz/fileserver-operator:X.Y.Z` and the chart
`oci://ghcr.io/wutz/charts/fileserver-operator:X.Y.Z`.

## License

Apache License 2.0. See [LICENSE](LICENSE).
