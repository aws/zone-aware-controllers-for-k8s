package podzone

import (
	"time"

	cache "k8s.io/client-go/tools/cache"
)

type PodZone struct {
	PodName string
	Zone    string
}

const (
	cacheTTL = time.Hour * 2
)

func NewCache() cache.Store {
	return cache.NewTTLStore(keyFunc, cacheTTL)
}

func keyFunc(obj interface{}) (string, error) {
	return obj.(PodZone).PodName, nil
}
