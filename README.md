# Zone Aware Controllers for K8s

## Introduction

Kubernetes controllers for zone (AZ) aware rollouts and disruptions.

## Controllers

### ZoneAwareUpdates (ZAU)

The ZoneAwareUpdate (ZAU) controller enables faster deployments for a StatefulSet whose pods are deployed across multiple availability zones. At each control loop, it applies zone-aware logic to check for pods in an old revision and deletes them so that they can be updated to a new revision.

#### Update Strategy

The controller exponentially increases the number of pods simultaneously deleted, deploying slowly at first and accelerating as confidence is gained in the new revision. For example, it will start by updating a single pod, then 2, then 4 and so on. The number of pods deleted in an iteration will never exceed the configured `MaxUnavailable` value.

The controller also never update pods from different zones at the same time, and when moving to subsequent zones it continues to increase the number of pods to be deleted until `MaxUnavailable` is reached.

After deleting pods, the controller will wait for them to transition to `Ready` state before updating the next set of pods.

When a rollback (or new rollout) is initiated before a deployment finishes, it is important to delete the most recently updated pods first to move away as fast as possible from a faulty revision. To achieve that, the controller always deletes pods in a specific order, using the zone ascending alphabetical order in conjunction with the pod decreasing ordinal order, as shown in the figure below:

```
             >>>>>----------------- Update Sequence (MaxUnavailable = 4) ------------------->

pod #   [[28], [27, 22], [19, 17, 15, 10], [8, 6, 1]], [[29, 26, 23, 20], [16, 14, 11, 7], [5, 2]], ...
        |                                           |  |                                         |
        '---------------- zone-1 -------------------'  '---------------- zone-2 -----------------'
```


Some applications don't necessarily need to have pods updated exponentially. For those, it's possible to disable exponential updates by setting the `ExponentialFactor` to zero.

```
          >>>>>---------- Update Sequence (MaxUnavailable = 4, ExponentialFactor = 0) -------->

pod #   [[28, 27, 22, 19], [17, 15, 10, 8], [6, 1]], [[29, 26, 23, 20], [16, 14, 11, 7], [5, 2]], ...
        |                                         |  |                                         |
        '---------------- zone-1 -----------------'  '---------------- zone-2 -----------------'
```

#### Usage

To have the rollout of a StatefulSet's pods coordinated by ZAU controller, the StatefulSet update strategy should be changed to [`OnDelete`](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/#update-strategies) and a `ZoneAwareUpdate` resource defined into the same namespace as the StatefulSet.

```yaml
apiVersion: zonecontrol.k8s.aws/v1
kind: ZoneAwareUpdate
metadata:
  name: <zau-name>
spec:
  statefulset: <sts-name>
  maxUnavailable: 2
```

`maxUnavailable` can be an absolute number or a percentage of total Pods. For example, in case your application is evenly distributed accross 3 zones, it's possible to update all pods at once in each zone by setting `maxUnavailable` to at leat 33% and exponentialFactor to 0:

```yaml
apiVersion: zonecontrol.k8s.aws/v1
kind: ZoneAwareUpdate
metadata:
  name: <zau-name>
spec:
  statefulset: <sts-name>
  maxUnavailable: 33%
  exponentialFactor: 0
```

It's also possible to specify the name of a Amazon CloudWatch aggregate alarm that will pause the rollout when in alarm state. This can be used to prevent deployments from preceeding in case of canary failures, for example.

```yaml
apiVersion: zonecontrol.k8s.aws/v1
kind: ZoneAwareUpdate
metadata:
  name: <zau-name>
spec:
  statefulset: <sts-name>
  maxUnavailable: 2
  pauseRolloutAlarm: <cw-aggregate-alarm-name>
  ignoreAlarm: false
```

### ZoneDisruptionBudgets (ZDB)

The ZoneDisruptionBudget (ZDB) admission webhook controller extends the PodDisruptionBudgets (PDB) concept, allowing multiple disruptions only if the pods being disrupted are in the same zone.

Similar to the k8s' [DisruptionController](https://github.com/kubernetes/kubernetes/blob/d7123a65248e25b86018ba8220b671cd483d6797/pkg/controller/disruption/disruption.go#L555), the ZoneDisruptionBudget (ZDB) Controller is responsible for watching for changes to ZDBs and for keeping their status up to date, checking at each control-loop (https://kubernetes.io/docs/concepts/architecture/controller/) which pods are unavailable to calculate the number of disruptions allowed per zone at any time.

A validation admission webhook is used to intercept requests to the eviction API, accepting or rejecting them based on ZDB's status, allowing multiple pods disruptions in zone-1, while blocking evictions from other zones.

#### Usage

A `ZoneDisruptionBudget` has three fields:

- A label selector `.spec.selector` to specify the set of pods to which it applies. This field is required.
- `.spec.maxUnavailable` that defines the maximun number of pods in the same zone that can be unavailable after the eviction. It can be either an absolute number or a percentage.

```yaml
apiVersion: zonecontrol.k8s.aws/v1
kind: ZoneDisruptionBudget
metadata:
  name: <zdb-name>
spec:
  selector: <pod-selector>
  maxUnavailable: 10%
```

## Installation

The controllers were built using the [kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) framework. The kubebuilder based `Makefile` is available to use for development and deployment.

To build and push the controllers image to your container registry:

```
make docker-build docker-push IMG=<image-url>
```

To deploy the controllers to the K8s cluster specified in `~/.kube/config`:

```
make deploy IMG=<image-url>
```

The controllers will be deployed to the `zone-aware-controllers-system` namespace by default. The namespace used can be changed in `./config/default/kustomization.yaml` file.

Finally, to undeploy the controllers:

```
make undeploy 
```

## Security

See [CONTRIBUTING](CONTRIBUTING.md#security-issue-notifications) for more information.

## License

This project is licensed under the Apache-2.0 License.