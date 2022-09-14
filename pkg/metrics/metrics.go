package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	opsv1 "github.com/aws/zone-aware-controllers-for-k8s/api/v1"
)

var (
	zdbMetricLabels         = []string{"namespace", "zdb"}
	zdbPerZoneMetricLabels  = []string{"namespace", "zdb", "zone"}
	zdbEvictionMetricLabels = []string{"status", "reason"}
	zauMetricLabels         = []string{"namespace", "zau"}
	zauPerZoneMetricLabels  = []string{"namespace", "zau", "zone"}

	currentHealth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zdb_status_current_healthy",
			Help: "Current number of healthy pods",
		},
		zdbPerZoneMetricLabels,
	)
	currentUnhealth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zdb_status_current_unhealthy",
			Help: "Current number of unhealthy pods",
		},
		zdbPerZoneMetricLabels,
	)
	zonesUnhealthy = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zdb_status_zones_unhealthy",
			Help: "Current number of unhealthy zones",
		},
		zdbMetricLabels,
	)
	desiredHealthy = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zdb_status_desired_healthy",
			Help: "Minimum desired number of healthy pods",
		},
		zdbPerZoneMetricLabels,
	)
	expectedPods = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zdb_status_expected_pods",
			Help: "Total number of pods counted by this disruption budget",
		},
		zdbPerZoneMetricLabels,
	)
	disruptionsAllowed = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zdb_status_disruptions_allowed",
			Help: "Number of pod disruptions that are currently allowed",
		},
		zdbPerZoneMetricLabels,
	)
	dryRunEnabled = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zdb_dryrun_enabled",
			Help: "Returns if dryRun is enabled or not",
		},
		zdbMetricLabels,
	)

	evictionStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zdb_eviction_status_reason",
			Help: "Describes the status reason for eviction requests",
		},
		zdbEvictionMetricLabels,
	)

	zauUpdateStep = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zau_status_update_step",
			Help: "Update/rollout step. It becomes zero when all pods are in the new revision and no rollout is in progress",
		},
		zauMetricLabels,
	)
	zauDeletedReplicas = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zau_status_deleted_replicas",
			Help: "Number of pods deleted in the last update step",
		},
		zauMetricLabels,
	)
	zauOldReplicas = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zau_status_old_replicas",
			Help: "Number of pods in an old revision",
		},
		zauPerZoneMetricLabels,
	)
	zauDryRunEnabled = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zau_dryrun_enabled",
			Help: "Returns if dryRun is enabled or not",
		},
		zauMetricLabels,
	)
	zauPausedRollout = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "zau_paused_rollout",
			Help: "Returns if rollout is paused or not",
		},
		zauMetricLabels,
	)
)

func init() {
	metrics.Registry.MustRegister(currentHealth, currentUnhealth, zonesUnhealthy, desiredHealthy, expectedPods,
		disruptionsAllowed, dryRunEnabled, evictionStatus, zauUpdateStep, zauDeletedReplicas,
		zauOldReplicas, zauDryRunEnabled, zauPausedRollout)
}

func PublishZdbStatusMetrics(zdb *opsv1.ZoneDisruptionBudget) {
	for zone := range zdb.Status.CurrentHealthy {
		currentHealth.WithLabelValues(zdb.Namespace, zdb.Name, zone).Set(float64(zdb.Status.CurrentHealthy[zone]))
	}

	zonesWithUnhealthyPods := 0
	for zone := range zdb.Status.CurrentUnhealthy {
		if zdb.Status.CurrentUnhealthy[zone] > 0 {
			zonesWithUnhealthyPods++
		}
		currentUnhealth.WithLabelValues(zdb.Namespace, zdb.Name, zone).Set(float64(zdb.Status.CurrentUnhealthy[zone]))
	}
	zonesUnhealthy.WithLabelValues(zdb.Namespace, zdb.Name).Set(float64(zonesWithUnhealthyPods))

	for zone := range zdb.Status.DesiredHealthy {
		desiredHealthy.WithLabelValues(zdb.Namespace, zdb.Name, zone).Set(float64(zdb.Status.DesiredHealthy[zone]))
	}
	for zone := range zdb.Status.ExpectedPods {
		expectedPods.WithLabelValues(zdb.Namespace, zdb.Name, zone).Set(float64(zdb.Status.ExpectedPods[zone]))
	}
	for zone := range zdb.Status.DisruptionsAllowed {
		disruptionsAllowed.WithLabelValues(zdb.Namespace, zdb.Name, zone).Set(float64(zdb.Status.DisruptionsAllowed[zone]))
	}

	dryRun := 0
	if zdb.Spec.DryRun {
		dryRun = 1
	}
	dryRunEnabled.WithLabelValues(zdb.Namespace, zdb.Name).Set(float64(dryRun))
}

func PublishEvictionStatusMetrics(response admission.Response, reason string) {
	if response.Allowed {
		evictionStatus.WithLabelValues("allowed", reason).Add(float64(1))
	} else {
		evictionStatus.WithLabelValues("denied", reason).Add(float64(1))
	}
}

func PublishZauStatusMetrics(zau *opsv1.ZoneAwareUpdate) {
	zauDeletedReplicas.WithLabelValues(zau.Namespace, zau.Name).Set(float64(zau.Status.DeletedReplicas))
	zauUpdateStep.WithLabelValues(zau.Namespace, zau.Name).Set(float64(zau.Status.UpdateStep))

	for zone := range zau.Status.OldReplicas {
		zauOldReplicas.WithLabelValues(zau.Namespace, zau.Name, zone).Set(float64(zau.Status.OldReplicas[zone]))
	}

	dryRun := 0
	if zau.Spec.DryRun {
		dryRun = 1
	}
	zauDryRunEnabled.WithLabelValues(zau.Namespace, zau.Name).Set(float64(dryRun))
	pausedRollout := 0
	if zau.Status.PausedRollout {
		pausedRollout = 1
	}
	zauPausedRollout.WithLabelValues(zau.Namespace, zau.Name).Set(float64(pausedRollout))
}
