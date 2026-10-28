# fileserver

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
- Skips volumes labelled `fileserver.wutz.dev/ignore=true`, so clients can keep a volume out of every file server.

## Install

The operator is published as a Helm chart and a container image:

```bash
helm install fileserver oci://ghcr.io/wutz/charts/fileserver \
  --version 0.3.0 \
  --namespace fileserver-system --create-namespace \
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
| `image.repository` / `image.tag` | `ghcr.io/wutz/fileserver` / chart `appVersion` | Operator image. |
| `replicas` | `1` | Operator replicas. Use more than one with `leaderElection: true`. |
| `leaderElection` | `false` | Enable leader election. |

See [`charts/fileserver/values.yaml`](charts/fileserver/values.yaml)
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

The Secret and object names, and the `app.kubernetes.io/managed-by=forgeai` labels are
kept from the forgeai platform. From 0.2.0 the API group is `fileserver.wutz.dev`.

From 0.3.0 the project is named `fileserver`: the chart, the image, the Go module and the
default namespace `fileserver-system`. The ignore label is `fileserver.wutz.dev/ignore=true`;
it replaces `forgeai.wutz.dev/trash`, which the operator no longer reads. forgeai sets the
new label on the volumes it trashes and relabels the ones trashed before 0.3.0.

Because the chart was renamed, `fileserver-operator` 0.2.0 cannot be upgraded in place.
Uninstall the `fileserver-operator` release first. That deletes its CRD, so it also deletes
every FileServer, and the Deployment, Service, Ingress and Secret those FileServers owned are
garbage-collected with them. Then install the `fileserver` chart. The Secret gets a newly
generated password, and forgeai recreates the `default` FileServer on the next use of the Files view.

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
`ghcr.io/wutz/fileserver:X.Y.Z` and the chart
`oci://ghcr.io/wutz/charts/fileserver:X.Y.Z`.

## License

Apache License 2.0. See [LICENSE](LICENSE).
