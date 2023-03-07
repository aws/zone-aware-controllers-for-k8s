//go:build integration

package zau

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/e2e-framework/klient/k8s"
)

var statefulsetManifest = []byte(`
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: web
spec:
  podManagementPolicy: Parallel
  updateStrategy:
    type: OnDelete
  selector:
    matchLabels:
      app: nginx
  serviceName: "nginx"
  replicas: 25
  template:
    metadata:
      labels:
        app: nginx
        version: "v0.1"
    spec:
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: DoNotSchedule
      containers:
        - name: nginx
          image: registry.k8s.io/nginx-slim:0.8
          ports:
            - containerPort: 80
              name: web
          readinessProbe:
            httpGet:
              scheme: HTTP
              path: /index.html
              port: 80
            periodSeconds: 3
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes:
      - ReadWriteOnce
      storageClassName: gp2
      resources:
        requests:
          storage: 1Gi
`)

func statefulsetImagePatch(image string) k8s.Patch {
	return k8s.Patch{
		PatchType: types.StrategicMergePatchType,
		Data: []byte(fmt.Sprintf(`
        {
          "spec": {
            "template": {
              "spec": {
                "containers": [{"name": "nginx", "image": "%s"}]
              }
            }
          }
        }`, image)),
	}
}

func pauseAlarmPatch(alarm string) k8s.Patch {
	return k8s.Patch{
		PatchType: types.MergePatchType,
		Data: []byte(fmt.Sprintf(`
		{
          "spec": {
            "pauseRolloutAlarm": "%v"
          }
		}`, alarm)),
	}
}

var zauManifest = []byte(`
apiVersion: zonecontrol.k8s.aws/v1
kind: ZoneAwareUpdate
metadata:
  name: zau
spec:
  statefulset: web
  maxUnavailable: 10%
  dryRun: false
`)
