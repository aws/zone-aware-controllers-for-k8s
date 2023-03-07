//go:build integration

package zdb

var zdbManifest = []byte(`
apiVersion: zonecontrol.k8s.aws/v1
kind: ZoneDisruptionBudget
metadata:
  name: zdb
spec:
  maxUnavailable: 10%
  dryRun: false
  selector:
    matchLabels:
      app: e2e
`)

var statefulsetManifest = []byte(`
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: web
spec:
  podManagementPolicy: Parallel
  selector:
    matchLabels:
      app: e2e
  serviceName: nginx
  replicas: 9
  template:
    metadata:
      labels:
        app: e2e
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
              name: e2e-pod
          readinessProbe:
            httpGet:
              scheme: HTTP
              path: /index.html
              port: 80
            periodSeconds: 3
`)
