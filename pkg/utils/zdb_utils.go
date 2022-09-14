package utils

import (
	"context"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	opsv1 "github.com/aws/zone-aware-controllers-for-k8s/api/v1"
)

func GetZdbForPod(c client.Client, logger logr.Logger, pod *v1.Pod) (*opsv1.ZoneDisruptionBudget, error) {
	if len(pod.GetLabels()) == 0 {
		return nil, nil
	}

	zdbList := &opsv1.ZoneDisruptionBudgetList{}
	if err := c.List(context.TODO(), zdbList, &client.ListOptions{Namespace: pod.GetNamespace()}); err != nil {
		return nil, err
	}

	var matchedZdbs []opsv1.ZoneDisruptionBudget
	for _, zdb := range zdbList.Items {
		labelSelector, err := metav1.LabelSelectorAsSelector(zdb.Spec.Selector)
		if err != nil {
			continue
		}
		if labelSelector.Empty() || !labelSelector.Matches(labels.Set(pod.GetLabels())) {
			continue
		}
		matchedZdbs = append(matchedZdbs, zdb)
	}

	if len(matchedZdbs) == 0 {
		return nil, nil
	}

	if len(matchedZdbs) > 1 {
		logger.Info("Pod matches multiple ZoneDisruptionBudgets. Choosing first matched zdb.", "pod", pod.GetName(), "zdb", matchedZdbs[0].GetName())
	}

	return &matchedZdbs[0], nil
}
