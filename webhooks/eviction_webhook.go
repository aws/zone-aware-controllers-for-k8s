/*
Copyright 2016 The Kubernetes Authors.
Modifications Copyright 2022 Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package webhook

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/zone-aware-controllers-for-k8s/pkg/utils"
	"github.com/go-logr/logr"
	admissionv1 "k8s.io/api/admission/v1"
	v1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta "k8s.io/api/policy/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apiserver/pkg/util/dryrun"
	"k8s.io/client-go/util/retry"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	opsv1 "github.com/aws/zone-aware-controllers-for-k8s/api/v1"
	"github.com/aws/zone-aware-controllers-for-k8s/pkg/metrics"
)

// The Pod Eviction Webhook is responsible for allow or deny pod evictions based on the ZDB status.
//
// The logic applied here was copied and adapted from the k8s' eviction API:
// https://github.com/kubernetes/kubernetes/blob/72a1dcb6e727/pkg/registry/core/pod/storage/eviction.go

//+kubebuilder:webhook:path=/pod-eviction-v1,mutating=true,failurePolicy=fail,groups="",resources=pods/eviction,verbs=create,versions=v1,name=eviction.zone-aware-controllers.svc,sideEffects=None,admissionReviewVersions={v1,v1beta1}

const (
	// Same max size used on PDBs:
	// https://github.com/kubernetes/kubernetes/blob/72a1dcb6e727/pkg/registry/core/pod/storage/eviction.go#L50
	MaxDisruptedPodSize = 2000
)

// EvictionsRetry is the retry for a conflict where multiple clients
// are making changes to the same resource.
var EvictionsRetry = wait.Backoff{
	Steps:    20,
	Duration: 500 * time.Millisecond,
	Factor:   1.0,
	Jitter:   0.1,
}

type PodEvictionHandler struct {
	Client  client.Client
	Logger  logr.Logger
	decoder *admission.Decoder
}

func (h *PodEvictionHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	response, reason := h.handler(ctx, req)
	metrics.PublishEvictionStatusMetrics(response, reason)
	return response
}

func (h *PodEvictionHandler) InjectDecoder(d *admission.Decoder) error {
	h.decoder = d
	return nil
}

func (h *PodEvictionHandler) handler(ctx context.Context, req admission.Request) (admission.Response, string) {
	// ignore non-create operations
	if req.AdmissionRequest.Operation != admissionv1.Create {
		h.Logger.Info("Pod non-CREATE operation allowed", "pod", req.Name, "resource", req.Resource, "subResource", req.SubResource)
		return admission.Allowed(""), "NoCreateRequest"
	}

	// ignore create operations other than subresource eviction
	if req.AdmissionRequest.SubResource != "eviction" {
		h.Logger.Info("Pod CREATE operation allowed", "pod", req.Name, "resource", req.Resource, "subResource", req.SubResource)
		return admission.Allowed(""), "NoEvictionRequest"
	}

	dryRun, err := h.getDryRunOption(req)
	if err != nil {
		return admission.Errored(http.StatusBadRequest, err), "GetDryRunOptionError"
	}

	pod := &v1.Pod{}
	key := types.NamespacedName{
		Namespace: req.AdmissionRequest.Namespace,
		Name:      req.AdmissionRequest.Name,
	}

	if err = h.Client.Get(ctx, key, pod); err != nil {
		h.Logger.Error(err, "Unable to fetch pod", "name", key.Name, "namespace", key.Namespace)
		if dryRun {
			h.Logger.Info("DryRun option enabled, allowing eviction request", "pod", key.Name)
			return admission.Allowed(""), "DryRun"
		}
		return admission.Errored(http.StatusInternalServerError, err), "GetPodError"
	}

	// Evicting a terminal pod should result in direct deletion of pod as it already caused disruption by the time we are evicting.
	// There is no need to check for zdb.
	if canIgnoreZDB(pod) {
		return admission.Allowed(""), "canIgnoreZdb"
	}

	// If the pod is not ready, it doesn't count towards healthy and we should not decrement
	if !utils.IsPodReady(pod) {
		h.Logger.Info("Pod is not ready, no need to check zdb", "pod", pod.Name)
		return admission.Allowed(""), "NotReadyPod"
	}

	zdb, err := utils.GetZdbForPod(h.Client, h.Logger, pod)
	if err != nil {
		h.Logger.Error(err, "Failed to get ZDB", "pod", pod.Name)
		if dryRun {
			h.Logger.Info("DryRun option enabled, allowing eviction request", "pod", pod.Name)
			return admission.Allowed(""), "DryRun"
		}
		return admission.Errored(http.StatusInternalServerError, err), "GetZdbError"
	}
	if zdb == nil {
		h.Logger.Info("No ZDB associated to the pod", "pod", pod.Name)
		return admission.Allowed(""), "NoZdb"
	}

	dryRun = dryRun || zdb.Spec.DryRun

	// If the pod is not ready, it doesn't count towards healthy and we should not decrement
	if isDisruptedPod(pod.Name, zdb) {
		h.Logger.Info("Pod disruption already recorded in zdb", "pod", pod.Name, "zdb", zdb.Name)
		return admission.Allowed(""), "AlreadyDisrupted"
	}

	refresh := false
	err = retry.RetryOnConflict(EvictionsRetry, func() error {
		if refresh {
			key = types.NamespacedName{
				Namespace: zdb.Namespace,
				Name:      zdb.Name,
			}
			if err = h.Client.Get(ctx, key, zdb); err != nil {
				return err
			}
		}

		if err = h.checkAndDecrement(ctx, pod, zdb, dryRun); err != nil {
			refresh = true
			return err
		}

		return nil
	})

	if err != nil {
		h.Logger.Error(err, "Denying pod eviction", "pod", pod.Name)
		if dryRun {
			h.Logger.Info("DryRun option enabled, allowing eviction request", "pod", pod.Name)
			return admission.Allowed(""), "DryRun"
		}
		return admission.Denied(fmt.Sprintf("denying pod eviction for %s", pod.Name)), "DeniedByZdb"
	}

	return admission.Allowed(""), "DisruptionAllowed"
}

func (h *PodEvictionHandler) checkAndDecrement(ctx context.Context, pod *v1.Pod, zdb *opsv1.ZoneDisruptionBudget, dryRun bool) error {
	if zdb.Status.ObservedGeneration < zdb.Generation {
		return errors.NewForbidden(
			opsv1.Resource("zonedisruptionbudget"),
			zdb.Name,
			fmt.Errorf("observed generation is not equals to zdb generation"),
		)
	}

	zone, err := h.getZone(ctx, pod)
	if err != nil {
		return errors.NewForbidden(
			opsv1.Resource("zonedisruptionbudget"),
			zdb.Name,
			err,
		)
	}

	if zdb.Status.DisruptionsAllowed[zone] < 0 {
		return errors.NewForbidden(
			opsv1.Resource("zonedisruptionbudget"),
			zdb.Name,
			fmt.Errorf("zdb disruptions allowed is negative"),
		)
	}

	if len(zdb.Status.DisruptedPods) > MaxDisruptedPodSize {
		return errors.NewForbidden(
			opsv1.Resource("zonedisruptionbudget"),
			zdb.Name,
			fmt.Errorf("DisruptedPods map too big - too many evictions not confirmed by ZDB controller"),
		)
	}

	if zdb.Status.DisruptionsAllowed[zone] == 0 {
		return errors.NewForbidden(
			opsv1.Resource("zonedisruptionbudget"),
			zdb.Name,
			fmt.Errorf("cannot evict pod as it would violate the zone disruption budget"),
		)
	}

	// If this is a dry-run, we don't need to go any further than that.
	if dryRun {
		return nil
	}

	zdb.Status.DisruptionsAllowed[zone]--
	if zdb.Status.DisruptedPods == nil {
		zdb.Status.DisruptedPods = make(map[string]metav1.Time)
	}

	// Eviction handler needs to inform the ZDB controller that it is about to delete a pod
	// so it should not consider it as available in calculations when updating Disruptions allowed.
	// If the pod is not deleted within a reasonable time limit PDB controller will assume that it won't
	// be deleted at all and remove it from DisruptedPod map.
	zdb.Status.DisruptedPods[pod.Name] = metav1.Time{Time: time.Now()}

	err = h.Client.Status().Update(ctx, zdb)
	if err != nil {
		return err
	}

	h.Logger.Info("ZDB disrupted pods updated", "spec", zdb.Spec, "status", zdb.Status, "pod", pod.Name)
	return nil
}

func canIgnoreZDB(pod *v1.Pod) bool {
	if pod.Status.Phase == v1.PodSucceeded || pod.Status.Phase == v1.PodFailed ||
		pod.Status.Phase == v1.PodPending || !pod.ObjectMeta.DeletionTimestamp.IsZero() {
		return true
	}
	return false
}

func isDisruptedPod(podName string, zdb *opsv1.ZoneDisruptionBudget) bool {
	_, ok := zdb.Status.DisruptedPods[podName]
	return ok
}

func (h *PodEvictionHandler) getZone(ctx context.Context, pod *v1.Pod) (string, error) {
	node := &v1.Node{}
	if err := h.Client.Get(ctx, types.NamespacedName{Name: pod.Spec.NodeName, Namespace: ""}, node); err != nil {
		return "", err
	}

	zone, ok := node.ObjectMeta.Labels[v1.LabelTopologyZone]
	if !ok {
		return "", fmt.Errorf("zone label not found for pod %q and node %q", pod.Name, node.Name)
	}

	return zone, nil
}

func (h *PodEvictionHandler) getDryRunOption(req admission.Request) (bool, error) {
	eviction := &policyv1.Eviction{}
	err := h.decoder.Decode(req, eviction)
	if err != nil {
		evictionBeta := &policyv1beta.Eviction{}
		err = h.decoder.Decode(req, evictionBeta)
		if err != nil {
			h.Logger.Error(err, "Failed to decode pod eviction", "resource", req.Resource, "subResource", req.SubResource)
			return false, err
		}
		if evictionBeta.DeleteOptions != nil {
			return dryrun.IsDryRun(evictionBeta.DeleteOptions.DryRun), nil
		}
		return false, nil
	}
	if eviction.DeleteOptions != nil {
		return dryrun.IsDryRun(eviction.DeleteOptions.DryRun), nil
	}
	return false, nil
}
