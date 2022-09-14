package podzone

import (
	"context"
	"sort"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	k8scache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Helper struct {
	client.Client
	Logger logr.Logger
	Cache  k8scache.Store
}

func (h *Helper) GetZonePodsMap(ctx context.Context, pods []*v1.Pod) map[string][]*v1.Pod {
	podZoneMap := map[string][]*v1.Pod{}
	for _, pod := range pods {
		var found bool
		zone, found := h.getPodZone(ctx, pod)
		if !found {
			continue
		}
		if _, ok := podZoneMap[zone]; ok {
			podZoneMap[zone] = append(podZoneMap[zone], pod)
		} else {
			podZoneMap[zone] = []*v1.Pod{pod}
		}
	}
	return podZoneMap
}

func (h *Helper) GetSortedZonesFromMap(zonePodsMap map[string][]*v1.Pod) []string {
	zones := make([]string, 0, len(zonePodsMap))
	for zone := range zonePodsMap {
		zones = append(zones, zone)
	}
	sort.Strings(zones)
	return zones
}

func (h *Helper) getPodZone(ctx context.Context, pod *v1.Pod) (string, bool) {
	node := &v1.Node{}
	if err := h.Get(ctx, types.NamespacedName{Name: pod.Spec.NodeName, Namespace: ""}, node); err != nil {
		if errors.IsNotFound(err) {
			h.Logger.Info("Node not found... trying cache", "pod", pod.Name)
		} else {
			h.Logger.Error(err, "Unable to get node... trying cache", "pod", pod.Name)
		}
		return h.getZoneFromCache(pod.Name)
	}

	zone, ok := node.ObjectMeta.Labels[v1.LabelTopologyZone]
	if !ok {
		h.Logger.Info("Zone label not found... trying cache", "node", node.GetName(), "pod", pod.Name)
		return h.getZoneFromCache(pod.Name)
	}

	podZone := PodZone{
		PodName: pod.Name,
		Zone:    zone,
	}
	if err := h.Cache.Add(podZone); err != nil {
		h.Logger.Error(err, "Failed to update PodZoneCache", "pod", pod.Name)
	}
	return zone, true
}

func (h *Helper) getZoneFromCache(podName string) (string, bool) {
	value, ok, err := h.Cache.GetByKey(podName)
	if err != nil {
		h.Logger.Error(err, "Failed to get zone information from cache", "pod", podName)
		return "", false
	}
	if ok {
		return value.(PodZone).Zone, true
	}
	h.Logger.Info("Zone information not in the cache", "pod", podName)
	return "", false
}
