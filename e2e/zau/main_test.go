//go:build integration

package zau

import (
	"os"
	"testing"

	"github.com/aws/zone-aware-controllers-for-k8s/e2e"

	operatorv1 "github.com/aws/zone-aware-controllers-for-k8s/api/v1"
	"k8s.io/client-go/kubernetes/scheme"
	log "k8s.io/klog/v2"
	"sigs.k8s.io/e2e-framework/pkg/env"
)

var testenv env.Environment

func TestMain(m *testing.M) {
	_testenv, err := e2e.MakeTestEnv()
	if err != nil {
		log.Fatalf("unable to create test environment: %v", err)
	}
	testenv = _testenv

	err = operatorv1.AddToScheme(scheme.Scheme)
	if err != nil {
		log.Fatal()
	}

	testenv.BeforeEachFeature(
		e2e.CreateTestNamespace,
		createStatefulSet,
		createZAU,
		waitForHealthyEnvironment,
		e2e.CreatePodZoneMap,
	)
	testenv.AfterEachFeature(
		e2e.DeleteTestNamespace,
	)

	// launch package tests
	os.Exit(testenv.Run(m))
}
