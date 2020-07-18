# configmap-to-disk

`configmap-to-disk` is a sidecar that synchronizes a value from a ConfigMap in the Kubernetes API to a path on disk.
This allows for instant reloading of files when they are updated in the API rather than having to wait for the Kubelet to update a volume mount.

[![Build Status](https://travis-ci.org/squat/configmap-to-disk.svg?branch=master)](https://travis-ci.org/squat/configmap-to-disk)
[![Go Report Card](https://goreportcard.com/badge/github.com/squat/configmap-to-disk)](https://goreportcard.com/report/github.com/squat/configmap-to-disk)

## Example Usage

Add the following container specification to a Pod in order to have the `configmap-to-disk` sidecar write the value of the key `bar` from ConfigMap `foo` to the path `/path/to/foo/bar`:

```yaml
- args:
  - --path=/path/to/foo/bar
  - --name=foo
  - --key=bar
  - --namespace=$(NAMESPACE)
  env:
  - name: NAMESPACE
    valueFrom:
      fieldRef:
        fieldPath: metadata.namespace
  image: squat/configmap-to-disk
  name: configmap-to-disk
  volumeMounts:
  - mountPath: /path/to/foo
    name: foo
```

## RBAC

`configmap-to-disk` requires permission to watch and list ConfigMaps in the target ConfigMap's namespace.
The following example RBAC resources could be used to provision the necessary permissions:

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: configmap-to-disk
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: list-watch-configmaps
rules:
- apiGroups:
  - ""
  resources:
  - configmaps
  verbs:
  - list
  - watch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: configmap-to-disk
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: list-watch-configmaps
subjects:
  - kind: ServiceAccount
    name: configmap-to-disk
```

## Usage

[embedmd]:# (tmp/help.txt)
```txt
Watch ConfigMaps in the API and write them to disk

Usage:
  configmap-to-disk [flags]

Flags:
  -h, --help                help for configmap-to-disk
      --key string          The ConfigMap key to read.
      --kubeconfig string   Path to kubeconfig. (default "/home/squat/src/kubeconeu2020/kubeconfig")
      --listen string       The address at which to listen for health and metrics. (default ":8080")
      --log-level string    Log level to use. Possible values: all, debug, info, warn, error, none (default "info")
      --name string         The ConfigMap name.
      --namespace string    The namespace to watch.
      --path string         Where to write the file.
```
