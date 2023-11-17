Isopod
======

[![CircleCI](https://circleci.com/gh/cruise-automation/isopod.svg?style=shield)](https://circleci.com/gh/cruise-automation/isopod)
[![Go Report Card](https://goreportcard.com/badge/github.com/cruise-automation/isopod)](https://goreportcard.com/report/github.com/cruise-automation/isopod)
[![GitHub Release](https://img.shields.io/github/release/cruise-automation/isopod.svg)](https://github.com/cruise-automation/isopod/releases)
[![GoDoc](https://godoc.org/github.com/cruise-automation/isopod?status.svg)](https://godoc.org/github.com/cruise-automation/isopod)

Isopod is an expressive DSL framework for Kubernetes configuration. Without
intermediate YAML artifacts, Isopod renders Kubernetes objects as [Protocol
Buffers](https://github.com/protocolbuffers/protobuf), so they are strongly
typed and consumed directly by the Kubernetes API.

With Isopod, configurations are scripted in
[Starlark](https://github.com/google/starlark-go), a Python dialect by Google
also used by [Bazel](https://github.com/bazelbuild/bazel) and
[Buck](https://github.com/facebook/buck) build systems. Isopod offers runtime
built-ins to access services and utilities such as Vault secret management,
Kubernetes apiserver, HTTP requester, Base64 encoder, and UUID generator, etc.
Isopod uses separate runtime for unit tests to mock all built-ins, providing the
test coverage not possible before.

A 5-min read, [this medium](https://medium.com/cruise/isopod-5ad7c565d350) post
explains the inefficiency of existing YAML templating tools when dealing with values
not statically known and complicated control logics such as loops and branches.
It also gives simple code examples to show why Isopod is an expressive,
hermetic, and extensible solution to configuration management in Kubernetes.

---

- [Isopod](#isopod)
- [Build](#build)
- [Main Entryfile](#main-entryfile)
  - [Clusters](#clusters)
      - [`gke()`](#gke)
      - [`onprem()`](#onprem)
  - [Addons](#addons)
  - [Generate Addons](#generate-addons)
- [Load Remote Isopod Modules](#load-remote-isopod-modules)
- [Built-ins](#built-ins)
  - [kube](#kube)
    - [Methods:](#methods)
      - [`kube.put`](#kubeput)
      - [`kube.delete`](#kubedelete)
      - [`kube.put_yaml`](#kubeput_yaml)
      - [`kube.get`](#kubeget)
      - [`kube.exists`](#kubeexists)
      - [`kube.from_str`, `kube.from_int`](#kubefrom_str-kubefrom_int)
  - [Vault](#vault)
    - [Methods:](#methods-1)
      - [`vault.read`](#vaultread)
      - [`vault.write`](#vaultwrite)
      - [`vault.exist`](#vaultexist)
  - [Helm](#helm)
    - [Methods:](#methods-2)
      - [`helm.apply`](#helmapply)
  - [Misc](#misc)
      - [`base64.{encode, decode}`](#base64encode-decode)
      - [`uuid.{v3, v4, v5}`](#uuidv3-v4-v5)
      - [`http.{get, post, patch, put, delete}`](#httpget-post-patch-put-delete)
      - [`hash.{sha256, sha1, md5}`](#hashsha256-sha1-md5)
      - [`sleep`](#sleep)
      - [`error`](#error)
- [Testing](#testing)
- [Dry Run Produces YAML Diffs](#dry-run-produces-yaml-diffs)
  - [Diff filtering](#diff-filtering)
- [License](#license)
- [Contributions](#contributions)

---

# Build

```shell
$ go version
go version go1.14 darwin/amd64
$ GO111MODULE=on go build
```

# Main Entryfile

Isopod will call the `clusters(ctx)` function in the main Starlark file to get a
list of target clusters. For each of such clusters, isopod will call
`addons(ctx)` to get a list of addons for configuration rollout.

Example:

```python
CLUSTERS = [
    onprem(env="dev", cluster="minikube", vaultkubeconfig="secret/path"),
    gke(
        env="prod",
        cluster="paas-prod",
        location="us-west1",
        project="cruise-paas-prod",
        use_internal_ip="false", # default to "false", which uses public endpoint
    ),
]

def clusters(ctx):
    if ctx.cluster != None:
        return [c for c in CLUSTERS if c.cluster == ctx.cluster]
    elif ctx.env != None:
        return [c for c in CLUSTERS if c.env == ctx.env]
    return CLUSTERS

def addons(ctx)
    return [
        addon("ingress", "configs/ingress.ipd", ctx),
    ]
```

## Clusters

The `ctx` argument to `clusters(ctx)` comes from the command line flag
`--context` to Isopod. This flag takes a comma-separated list of `foo=bar` and
makes these values available in Starlark as `ctx.foo` (which gives `"bar"`).
Currently Isopod supports the following clusters, and could easily be
extended to cover other Kubernetes vendors, such as EKS and AKS.

#### `gke()`

Represents a Google Kubernetes Engine. Authenticates using Google Cloud Service Account Credentials or Google Default Application Credentials. Requires the `cluster`, `location` and `project` fields, while optionally takes `use_internal_ip` field to connect API server via private endpoint. Additional fields are allowed.

#### `onprem()`

Represents an on-premise or self-managed Kubernetes cluster. Authenticates using the `kubeconfig` file or Vault path containing the `kubeconfig`. No fields are required, though setting the `vaultkubeconfig` field to the path in Vault where the KubeConfig exists is necessary to utilize this auth method.


## Addons

The `ctx` argument to `addons(ctx)` contains all fields of the chosen cluster. For example, say the cluster is

```python
gke(
    env="prod",
    cluster="paas-prod",
    location="us-west1",
    project="cruise-paas-prod",
    use_internal_ip="false", # default to "false", which uses public endpoint
),
```

Then, each addon may access the cluster information as `ctx.env` to get `"prod"`
and `ctx.location` to get `"us-west1"`. Accessing nonexistant attribute `ctx.foo` will get `None`.

Each addon is represented using the `addon()` Starlark built-in, which takes
three arguments, for example `addon("name", "entry_file.ipd", ctx)`. The first
argument is the addon name, used by the `--match_addon` feature. The third
is optional and represents the `ctx` input to `addons(ctx)` to make the cluster
attributes available to the addon. Each addon must implement `install(ctx)` and
`remove(ctx)` functions.

More advanced examples can be found in the [examples](examples) folder.

Example Nginx addon:

```python
appsv1 = proto.package("k8s.io.api.apps.v1")
corev1 = proto.package("k8s.io.api.core.v1")
metav1 = proto.package("k8s.io.apimachinery.pkg.apis.meta.v1")

def install(ctx):
    metadata = metav1.ObjectMeta(
        name="nginx",
        namespace="example",
        labels={"app": "nginx"},
    )

    nginxContainer = corev1.Container(
        name=metadata.name,
        image="nginx:1.15.5",
        ports=[corev1.ContainerPort(containerPort=80)],
    ),

    deploySpec = appsv1.DeploymentSpec(
        replicas=3,
        selector=metav1.LabelSelector(matchLabels=metadata.labels),
        template=corev1.PodTemplateSpec(
            metadata=metadata,
            spec=corev1.PodSpec(
                containers=[nginxContainer],
            ),
        ),
    )

    kube.put(
        name=metadata.name,
        namespace=metadata.namespace,
        data=[appsv1.Deployment(
            metadata=metav1.ObjectMeta(name=metadata.name),
            spec=deploySpec,
        )],
    )
```

## Generate Addons

You might come from a place where you have a yaml file, but you want to derive an isopod addon from it. It can be
cumbersome to re-write huge yaml files in Starlark. So isopod offers a convenience command to generate the Starlark code
based on a yaml or json input file containing any [kubernetes API object](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/):

```bash
isopod generate runtime/testdata/clusterrolebinding.yaml > addon.ipd
```

For now all `k8s.io` resources are supported.


# Load Remote Isopod Modules

Similar to Bazel's `WORKSPACE` file, the `isopod.deps` file allows you to define remote
and versioned git modules to import to local modules. For example,

```python
git_repository(
    name="isopod_tools",
    commit="dbe211be57bc27b947ab3e64568ecc94c23a9439",
    remote="https://github.com/cruise-automation/isopod.git",
)
```

To import remote modules, use `load("@target_name//path/to/file", "foo", "bar")`,
for example,

```python
load("@isopod_tools//examples/helpers.ipd",
     "health_probe", "env_from_field", "container_port")

...
spec=corev1.PodSpec(
    containers=[corev1.Container(
        name="nginx-ingress-controller",
        image="quay.io/kubernetes-ingress-controller/nginx-ingress-controller:0.22.0",
        env=[
            env_from_field("POD_NAME", "metadata.name"),
            env_from_field("POD_NAMESPACE", "metadata.namespace"),
        ],
        livenessProbe=health_probe(10254),
        readinessProbe=health_probe(10254),
        ports=[
            container_port("http", 80),
            container_port("https", 443),
            container_port("metrics", 10254),
        ],
    )],
)
```

To import remote addon files, use `addon("addon_name", ""@addon_name//path/to/file", ctx)`,
for example,
```python
isopod.deps:

git_repository(
    name="versioned_addon",
    commit="1.0.0",
    remote="https://github.com/cruise-automation/addon.git",
)
...

main.ipd:

def addons(ctx):
    if ctx.cluster == None:
        error("`ctx.cluster' not set")
    if ctx.foobar != None:
        error("`ctx.foobar' must be `None', got: {foobar}".format(
            foobar=ctx.foobar))

    return [
        addon("addon_name", "@addon_name//addon/addon.ipd", ctx),
    ]
```

By default Isopod uses `$(pwd)/isopod.deps`, which you can override with `--deps` flag.

# Built-ins

Built-ins are pre-declared packages available in Isopod runtime. Typically they
perform I/O to Kubernetes, Vault, GCP and other resources but could be used for
break-outs into other operations not supported by the main Starlark interpreter.

Currently these build-ins are supported:

## kube

Built-in for managing Kubernetes objects.

### Methods:

#### `kube.put`

Updates (creates if it doesn't already exist) object in Kubernetes.

```python
kube.put(
    name = "nginx-role",
    namespace = "nginx-ingress",
    # Optional Kubernetes API Group parameter. If not set, will attempt to
    # deduce the group from message type but since Kubernetes API Group names
    # are highly irregular, this may fail.
    api_group = 'rbac.authorization.kubernetes.io',
    data = [
        rbacv1.Role(),
    ],
)
```

Supported args:
  + `name` - Name (`.metadata.name`) of the resource
  + `namespace` (Optional) - Namespace (`.metadata.namespace`) of the resource
  + `api_group` (Optional) - API group of the resource. If not provided,
     Isopod runtime will attempt to deduce the resource from
     just Proto type name which is unreliable. It is recommended to set this
     for all objects outside of `core` group. Optionally, version can also be
     specified after a `/`, example:
     + `apiextensions.k8s.io` - specify the group only, version is implied from Proto or from runtime.
     + `apiextensions.k8s.io/v1` - specify both group and version.
  + `subresource` (Optional) - A subresource specifier (e.g `/status`).
  + `data` - A list of Protobuf definitions of objects to be created.

---

#### `kube.delete`

Deletes object in Kubernetes.

```python
# kwarg key is resource name, value is <namespace>/<name> (just <name> for
# non-namespaced resources).
kube.delete(deployment="default/nginx")
# api_group can optionally be provided to remove ambuguity (if multiple
# resources by the same name exist in different API Groups or different versions).
kube.delete(clusterrole="nginx", api_group = "rbac.authorization.k8s.io/v1")
```

---

####  `kube.put_yaml`

Same as `put` but for YAML/JSON data. To be used for CRDs and other custom
types. `kube.put` usage is preferred for the standard set of Kubernetes types.

```python
ark_config = """
apiVersion: ark.heptio.com/v1
kind: Config
metadata:"
  namespace: ark-backup
  name: default
backupStorageProvider:
  name: gcp
  bucket: test-ark-backup
persistentVolumeProvider:
  name: gcp
"""

kube.put_yaml(
    name = "ark-config",
    namespace = "backup",
    data = [ark_config])

# Alternatively render from native Starlark struct object via JSON:
ark_config = struct(
    apiVersion = "ark.heptio.com/v1",
    kind = "Config",
    metadata = struct(
        name = "ark-backup",
        namespace = "default",
    ),
    backupStorageProvider = struct(
        name = "gcp",
        bucket = "test-ark-backup",
    ),
    persistentVolumeProvider = struct(
        name = "gcp",
    ),
)

kube.put_yaml(
    name = "ark-config",
    namespace = "backup",
    data = [ark_config.to_json()])
```

---

#### `kube.get`

Reads object from API Server. If `wait` argument is set to duration (e.g `10s`)
will block until the object is successfully read or timer expires. If
`json=True` optional argument is provided, will render object as unstructured
JSON represented as Starlark `dict` at top level. This is useful for CRDs as
they typically do not support Protobuf representation.

```python
# Wait 60s for Service Account token secret.
secret = kube.get(secret=namespace+"/"+serviceaccount.secrets[0].name, wait="60s")

# Get ClusterRbacSyncConfig CRD.
cadmin = kube.get(clusterrbacsyncconfig="cluster-admin",
                  api_group="rbacsync.getcruise.com",
                  json=True)
```

It is also possible to receive a list of kubernetes objects. They can be filtered
as defined in the [API documentation](https://raw.githubusercontent.com/kubernetes/kubernetes/master/api/openapi-spec/swagger.json).

```python
# Get all pods in namespace kube-system.
pods = kube.get(pod="kube-system/")

# Get all pods with label component=kube-apiserver
pods = kube.get(pod="kube-system/?labelSelector=component=kube-apiserver")
```

#### `kube.exists`

Checks whether a resource exists. If `wait` argument is set to duration (e.g
`10s`) will block until the object is successfully read or timer expires.

```python
# Assert that the resource doesn't exist.
e = kube.exists(secret=namespace+"/"+serviceaccount.secrets[0].name, wait="10s")
assert(e != True, "Fail: resource shouldn't exist")
```

---

#### `kube.from_str`, `kube.from_int`
Convert Starlark `string` and `int` types to corresponding `*instr.IntOrString`
protos.

```python
appsv1.RollingUpdateDaemonSet(
    maxUnavailable = kube.from_str("10%"),
)
```


## Vault

Vault break-out allows reading/writing values from Enterprise Vault.

### Methods:

#### `vault.read`

Reads data from Vault path as Starlark dict

#### `vault.write`

Writes kwargs to Vault path

#### `vault.exist`

Checks if path exists in Vault

Example usage:

```python
if not vault.exist("secret/lidar/stuff"):
    vault.write("secret/lidar/stuff", w1="hello", w2="world!")

data = vault.read("secret/infra/myapp")
print(data["w1"] + " " + data["w2"])
```

## Helm

Helm built-in renders Helm charts and applies the resource manifest changes.

### Methods:

#### `helm.apply`

Applies resource changes.

```python
globalValues = """
global:
    priorityClassName: "cluster-critical"
"""
pilotValues = """
pilot:
    replicaCount: 3
    image: docker.io/istio/pilot:v1.2.3
    traceSampling: 50.0
"""
pilotOverlayValues = {
    "pilot": {
        "traceSampling": 100.0,
    }
}

helm.apply(
    release_name = "istio-pilot",
    chart = "//charts/istio/istio-pilot",
    namespace = "istio-system",
    values = [
        yaml.unmarshal(globalValues),
        yaml.unmarshal(pilotValues),
        pilotOverlayValues
    ]
)
```

Supported args:
+ `release_name` - Release Name for the Helm chart.
+ `chart` - Source Path of the chart. This can be a full path or a path relative
  to the working directory. Having a leading double-slash (//) will make it
  relative path.
+ `namespace` (Optional) - Namespace (`.metadata.namespace`) of the resources
+ `values` (Optional) - A list of Starlark Values used as input values for the
   charts. The ordering of a list matters, and the elements get overridden by
   the trailing values.


## Misc

Various other utilities are available as Starlark built-ins for convenience:

#### `base64.{encode, decode}`

Translate string values to/from base64

#### `uuid.{v3, v4, v5}`

Produce corresponding flavor of UUID values

#### `http.{get, post, patch, put, delete}`

Sends corresponding HTTP request to specified url. Returns response body as
`string`, if present. Errors out on non-2XX response code. Will follow redirects
(stops after 10 consecutive requests).

Arguments:
  - `url` - URL to send request to (required).
  - `headers` - optional header `dict` (values are either `string` for
    single-value headers or `list` for multiple-value headers).
  - `data` - optionally send data in the body of the request (takes `string`).

#### `hash.{sha256, sha1, md5}`

Returns an integer hash value. Useful applied to an env var for forcing a
redeploy when a config or secret changes.

#### `sleep`

Pauses execution for specified duration (requires Go duration `string`).

#### `error`

Interrupts execution and return error to the user (requires `string` error
message).


# Testing

`isopod test` command allows addon creators to write hermetic unit tests on
their addons.

Unit tests must be contained inside files with a `_test.ipd` suffix and Isopod
runtime will call every top-level method defined in that file as a separate
test, execute it and report the result.

Built-in modules that allow external access (like `kube` and `vault`) are
stubbed (faked) out in unit test mode so that tests are hermetic.

Intended pattern is to import the addon config files from the test, then call
their methods and test the results with `assert` built-in (only supported in
test mode).

Example test:

```python
# Load ingress addon config and expose its "install" method.
load("testdata/ingress.ipd", "install")

def test_install(t):
    # Test setup code.
    vault.write("secret/car/cert", crt="foobar")
    t.ctx.namespace = "foobar"

    # Call method we are testing (creates namespace from context).
    install(t.ctx)

    # Now extract data from our fake "kube" module and verify our tests
    # conditions.
    ns = kube.get(namespace="foobar")
    assert(ns.metadata.name == "foobar", "fail")
    assert(ns.metadata.labels["foo"] == "bar", "fail")
```

The test command is designed to mimic standard `go test`. As such you can
execute all test in subtree by running `isopod test path/...`, all test in a
directory by running `isopod test path/` and all tests from a current working
subtree by running just `isopod test`.


# Dry Run Produces YAML Diffs

Knowledge regarding the intended actions of any specification change is crucial
for migration and everyday configuration updates. It prevents accidental removal
of the critical fields that is otherwise uncatchable with just the new set of
configurations.

In dry run mode, Isopod not only verifies the legitimacy of the Starlark scripts
but also informs the intended actions of the configuration change, by presenting
the YAML diff between live objects in cluster and the generated configurations
call "head". The result looks like the following.

```diff
*** service.v1 example/nginx ***
--- live
+++ head
@@ -14,8 +14,9 @@
     port: 80
     targetPort: 80
   selector:
     app: nginx
   clusterIP: 192.168.17.77
-  type: ClusterIP
+  type: NodePort
   sessionAffinity: None
+  externalTrafficPolicy: Cluster
```

## Diff filtering

Many fields are managed by controllers and updated at runtime, which means they
don't match the initially specified resource definition. In order to reduce noise
when evaluating whether a dry-run is safe to apply, some filtering is performed on
the current and requested resource definitions.

By default, Isopod attempts to apply schema defaults and filter fields that are
set by built-in kubernetes controllers at runtime.

In addition to the default filters, Isopod users may specify filters in two ways,
individually using `--kube_diff_filter` or in bulk with `--kube_diff_filter_file`.

Individual Filters Example:

```
$ isopod \
  --vault_token "${vault_token}" \
  --context "cluster=${cluster}" \
  --dry_run --nospin \
  --kube_diff_filter 'metadata.creationTimestamp' \
  --kube_diff_filter 'metadata.annotations["isopod.getcruise.com/context"]' \
  --kube_diff_filter 'metadata.annotations["deployment.kubernetes.io/revision"]' \
  --kube_diff_filter 'metadata.annotations["deprecated.daemonset.template.generation"]' \
  --kube_diff_filter 'metadata.annotations["autoscaling.alpha.kubernetes.io/conditions"]' \
  --kube_diff_filter 'metadata.annotations["cloud.google.com/neg-status"]' \
  --kube_diff_filter 'metadata.annotations["runscope.getcruise.com/api-test-ids"]' \
  --kube_diff_filter 'spec.template.spec.serviceAccount' \
  --kube_diff_filter 'spec.jobTemplate.spec.template.spec.serviceAccount' \
  install \
  "${DEFAULT_CONFIG_PATH}"
```

Bulk Filters Example:

```
$ cat > filters.txt <<EOF
metadata.creationTimestamp
metadata.annotations["isopod.getcruise.com/context"]
metadata.annotations["deployment.kubernetes.io/revision"]
metadata.annotations["deprecated.daemonset.template.generation"]
metadata.annotations["autoscaling.alpha.kubernetes.io/conditions"]
metadata.annotations["cloud.google.com/neg-status"]
metadata.annotations["runscope.getcruise.com/api-test-ids"]
spec.template.spec.serviceAccount
spec.jobTemplate.spec.template.spec.serviceAccount
EOF
$ isopod \
  --vault_token "${vault_token}" \
  --context "cluster=${cluster}" \
  --dry_run --nospin \
  --kube_diff_filter_file "filters.txt" \
  install \
  "${DEFAULT_CONFIG_PATH}"
```


# License

Copyright 2020 Cruise LLC

Licensed under the [Apache License Version 2.0](LICENSE) (the "License");
you may not use this project except in compliance with the License.

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.


# Contributions

Contributions are welcome! Please see the agreement for contributions in
[CONTRIBUTING.md](CONTRIBUTING.md).

Commits must be made with a Sign-off (`git commit -s`) certifying that you
agree to the provisions in [CONTRIBUTING.md](CONTRIBUTING.md).
