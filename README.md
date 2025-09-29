# cloud-autoscale-controller

[![release](https://img.shields.io/github/release/DoodleScheduling/cloud-autoscale-controller/all.svg)](https://github.com/DoodleScheduling/cloud-autoscale-controller/releases)
[![release](https://github.com/DoodleScheduling/cloud-autoscale-controller/actions/workflows/release.yaml/badge.svg)](https://github.com/DoodleScheduling/cloud-autoscale-controller/actions/workflows/release.yaml)
[![report](https://goreportcard.com/badge/github.com/DoodleScheduling/cloud-autoscale-controller)](https://goreportcard.com/report/github.com/DoodleScheduling/cloud-autoscale-controller)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/DoodleScheduling/cloud-autoscale-controller/badge)](https://api.securityscorecards.dev/projects/github.com/DoodleScheduling/cloud-autoscale-controller)
[![Coverage Status](https://coveralls.io/repos/github/DoodleScheduling/cloud-autoscale-controller/badge.svg?branch=master)](https://coveralls.io/github/DoodleScheduling/cloud-autoscale-controller?branch=master)
[![license](https://img.shields.io/github/license/DoodleScheduling/cloud-autoscale-controller.svg)](https://github.com/DoodleScheduling/cloud-autoscale-controller/blob/master/LICENSE)

Running cloud workload can be expensive, especially on testing, qa and prod like environments. 
Usually these resource are not required all the time and will consume your wallet for no reason.

This controller can scale and/or suspend cloud resources according the state of running pods or usage.
If a governing resource, for example a `AWSRDSInstance` detects that no pods are running which match any of the selectors in `.spec.scaleToZero` the 
referenced AWS RDS instance will be temporarily terminated. If any pod starts given the selectors the instance will be resumed again.

This approach can work great in environments which already have automated pod scale down. Especially with serverless workloads (scale to zero) and controllers like [k8s-pause](https://github.com/DoodleScheduling/k8s-pause).

This controller can be compared with tools like HPA and [Keda](https://keda.sh) but instead scaling pods on kubernetes it uses pod stats to scale resources pods
rely on.

## Supported resources

### AWS RDS

If no pods are running matching either `app: backend` or `app: another-rds-client` the aws instance named `rds-myname` will be terminated after
a grace period of 5 minutes.

**Note**: If no grace period is set the instance will be terminated immediately after the condition ScaledToZero is set to `True`.

```yaml
apiVersion: cloudautoscale.infra.doodle.com/v1beta1
kind: AWSRDSInstance
metadata:
  name: rds-myname
spec:
  instanceName: rds-myname # If instanceName is not set .metadata.name will be used
  region: eu-central-2
  gracePeriod: 5m
  interval: 15m
  secret:
    name: aws-credentials
  scaleToZero:
  - matchLabels:
      app: backend
  - matchLabels:
      app: another-rds-client
---
apiVersion: v1
data:
  AWS_ACCESS_KEY_ID: c2VjcmV0=
  AWS_SECRET_ACCESS_KEY: c2VjcmV0
kind: Secret
metadata:
  name: aws-credentials
type: Opaque
```

### MongoDB Atlas

If no pods are running matching either `app: backend` or `app: another-mongodb-client` the atlas cluster named `atlas-myname` will be terminated after
a grace period of 5 minutes.

```yaml
apiVersion: cloudautoscale.infra.doodle.com/v1beta1
kind: MongoDBAtlasCluster
metadata:
  name: atlas-myname
spec:
  clusterName: atlas-myname # If clusterName is not set .metadata.name will be used
  gracePeriod: 5m
  interval: 15m
  groupID: xxxx
  secret:
    name: atlas-credentials
  scaleToZero:
  - matchLabels:
      app: backend
  - matchLabels:
      app: another-mongodb-client
---
apiVersion: v1
data:
  privateKey: c2VjcmV0=
  publicKey: c2VjcmV0=
kind: Secret
metadata:
  name: atlas-credentials
type: Opaque
```

### Neo4j Aura

```
apiVersion: cloudautoscale.infra.doodle.com/v1beta1
kind: Neo4jAuraInstance
metadata:
  name: neo-myname
spec:
  instanceName: my-aura-instance # If instanceName is not set .metadata.name will be used
  gracePeriod: 5m
  interval: 15m
  secret:
    name: neo4j-project-admin
  scaleToZero:
  - matchLabels:
      app: backend
  - matchLabels:
      app: another-neo4j-client
---
apiVersion: v1
data:
  clientID: c2VjcmV0=
  clientSecret: c2VjcmV0=
kind: Secret
metadata:
  name: neo4j-project-admin
type: Opaque
``

## Observe reconciliation

Each resource reports various conditions in `.status.condtions` which will give the necessary insight about the 
current state of the resource.
Namely there are the conditions `Ready` and `ScaledToZero`.
ScaledToZero will give insigh whether the target rules have pods elected as up or down and Ready gives insight about
the reconcililation status itself.

```yaml
status:
  conditions:
  - lastTransitionTime: "2023-11-30T12:01:52Z"
    message: random cloud error
    observedGeneration: 32
    reason: ReconciliationFailed
    status: "False"
    type: Ready
  - lastTransitionTime: "2023-12-11T14:03:31Z"
    message: selector matches at least one running pod
    observedGeneration: 3
    reason: PodsRunning
    status: "False"
    type: ScaledToZero
```

## Installation

### Helm

Please see [chart/cloud-autoscale-controller](https://github.com/DoodleScheduling/cloud-autoscale-controller/tree/master/chart/cloud-autoscale-controller) for the helm chart docs.

### Manifests/kustomize

Alternatively you may get the bundled manifests in each release to deploy it using kustomize or use them directly.

## Configuration
The controller can be configured using cmd args:
```
--concurrent int                            The number of concurrent KeycloakRealm reconciles. (default 4)
--enable-leader-election                    Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.
--graceful-shutdown-timeout duration        The duration given to the reconciler to finish before forcibly stopping. (default 10m0s)
--health-addr string                        The address the health endpoint binds to. (default ":9557")
--insecure-kubeconfig-exec                  Allow use of the user.exec section in kubeconfigs provided for remote apply.
--insecure-kubeconfig-tls                   Allow that kubeconfigs provided for remote apply can disable TLS verification.
--kube-api-burst int                        The maximum burst queries-per-second of requests sent to the Kubernetes API. (default 300)
--kube-api-qps float32                      The maximum queries-per-second of requests sent to the Kubernetes API. (default 50)
--leader-election-lease-duration duration   Interval at which non-leader candidates will wait to force acquire leadership (duration string). (default 35s)
--leader-election-release-on-cancel         Defines if the leader should step down voluntarily on controller manager shutdown. (default true)
--leader-election-renew-deadline duration   Duration that the leading controller manager will retry refreshing leadership before giving up (duration string). (default 30s)
--leader-election-retry-period duration     Duration the LeaderElector clients should wait between tries of actions (duration string). (default 5s)
--log-encoding string                       Log encoding format. Can be 'json' or 'console'. (default "json")
--log-level string                          Log verbosity level. Can be one of 'trace', 'debug', 'info', 'error'. (default "info")
--max-retry-delay duration                  The maximum amount of time for which an object being reconciled will have to wait before a retry. (default 15m0s)
--metrics-addr string                       The address the metric endpoint binds to. (default ":9556")
--min-retry-delay duration                  The minimum amount of time for which an object being reconciled will have to wait before a retry. (default 750ms)
--watch-all-namespaces                      Watch for resources in all namespaces, if set to false it will only watch the runtime namespace. (default true)
--watch-label-selector string               Watch for resources with matching labels e.g. 'sharding.fluxcd.io/shard=shard1'.
```
