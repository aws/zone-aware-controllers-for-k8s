//go:build integration

package e2e

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/aws-iam-authenticator/pkg/token"
	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

// MakeTestEnv creates an e2e test environment using the kubeconfig located at $KUBECONFIG
// or defaults to an EKS test environment built from environment variables
func MakeTestEnv() (env.Environment, error) {
	awsconf, err := makeAWSConfig()
	if err != nil {
		return nil, err
	}

	// use KUBECONFIG if specific, otherwise assume EKS environment
	var environment env.Environment
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig != "" {
		environment = env.NewWithKubeConfig(kubeconfig)
	} else {
		client, err := makeEKSClient(awsconf)
		if err != nil {
			return nil, err
		}
		conf := envconf.New().WithClient(client)
		environment = env.NewWithConfig(conf)
	}

	// store aws conf for building service clients in tests where necessary
	ctx := contextWithAWSConfig(context.Background(), awsconf)

	return environment.WithContext(ctx), nil
}

func makeAWSConfig() (aws.Config, error) {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return aws.Config{}, fmt.Errorf("unable to load default config: %v", err)
	}

	return cfg, nil
}

func makeEKSClient(awsconf aws.Config) (klient.Client, error) {
	var (
		clusterAdminRole = os.Getenv("CLUSTER_ADMIN_ROLE")
		clusterName      = os.Getenv("CLUSTER_NAME")
	)

	if clusterAdminRole == "" || clusterName == "" {
		return nil, fmt.Errorf(
			"required environment variable not set:"+
				"CLUSTER_ADMIN_ROLE=%s\n"+
				"CLUSTER_NAME=%s\n",
			clusterAdminRole, clusterName)
	}

	eksctl := eks.NewFromConfig(awsconf)
	cluster, err := eksctl.DescribeCluster(context.Background(), &eks.DescribeClusterInput{
		Name: &clusterName,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to describe EKS cluster: %s", err)
	}

	gen, err := token.NewGenerator(true, false)
	if err != nil {
		return nil, fmt.Errorf("unable to setup token generator: %s", err)
	}

	tok, err := gen.GetWithRole(clusterName, clusterAdminRole)
	if err != nil {
		return nil, fmt.Errorf("unable to generate EKS token: %s", err)
	}

	ca, err := base64.StdEncoding.DecodeString(*cluster.Cluster.CertificateAuthority.Data)
	if err != nil {
		return nil, fmt.Errorf("unable to decode EKS certificate data: %s", err)
	}

	conf, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{},
		&clientcmd.ConfigOverrides{
			AuthInfo: api.AuthInfo{
				Token: tok.Token,
			},
			ClusterInfo: api.Cluster{
				Server:                   *cluster.Cluster.Endpoint,
				CertificateAuthorityData: ca,
			},
		},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to create k8s client config: %s", err)
	}

	client, err := klient.New(conf)
	if err != nil {
		return nil, err
	}

	return client, nil
}

// CreatePodZoneMap indexes all pods by zone and stores the index in context for faster lookup in tests
func CreatePodZoneMap(ctx context.Context, cfg *envconf.Config, t *testing.T, _ features.Feature) (context.Context, error) {
	var pods corev1.PodList
	if err := cfg.Client().Resources(NamespaceFromContext(ctx)).List(ctx, &pods); err != nil {
		return ctx, fmt.Errorf("listing pods: %v", err)
	}

	podZoneMap := make(map[string]string)
	for _, pod := range pods.Items {
		zone, err := GetPodZone(ctx, cfg, pod)
		if err != nil {
			return ctx, fmt.Errorf("getting pod zone: %v", err)
		}
		podZoneMap[pod.Name] = zone
	}

	return contextWithPodZoneMap(ctx, podZoneMap), nil
}

// CreateTestNamespace creates a namespace with a random name. It is stored in the context
// so that the deleteTestNamespace routine can look it up and delete it.
func CreateTestNamespace(ctx context.Context, cfg *envconf.Config, t *testing.T, _ features.Feature) (context.Context, error) {
	ns := corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: envconf.RandomName("e2e", 32),
		},
	}
	t.Logf("creating namespace: %v", ns.Name)
	return ContextWithNamespace(ctx, ns.Name), cfg.Client().Resources().Create(ctx, &ns)
}

// DeleteTestNamespace looks up the test namespace from ctx and deletes it.
func DeleteTestNamespace(ctx context.Context, cfg *envconf.Config, t *testing.T, _ features.Feature) (context.Context, error) {
	ns := corev1.Namespace{}
	ns.Name = NamespaceFromContext(ctx)
	t.Logf("deleting namespace: %v", ns.Name)
	return ctx, cfg.Client().Resources().Delete(ctx, &ns)
}

func ContextWithNamespace(ctx context.Context, ns string) context.Context {
	return context.WithValue(ctx, "test-namespace", ns)
}

func NamespaceFromContext(ctx context.Context) string {
	return ctx.Value("test-namespace").(string)
}

func contextWithPodZoneMap(ctx context.Context, podZoneMap map[string]string) context.Context {
	return context.WithValue(ctx, "pod-zone-map", podZoneMap)
}

// ZonesFromContext returns a list of all unique availability zones in the cluster
func ZonesFromContext(ctx context.Context) []string {
	index := ctx.Value("pod-zone-map").(map[string]string)

	var result []string
	added := make(map[string]bool)
	for _, zone := range index {
		if !added[zone] {
			result = append(result, zone)
			added[zone] = true
		}
	}

	return result
}

// PodZoneFromContext returns the zone where the pod with the given name is scheduled
func PodZoneFromContext(ctx context.Context, podName string) (string, bool) {
	zone, found := ctx.Value("pod-zone-map").(map[string]string)[podName]
	return zone, found
}

// ZonePodsFromContext returns a grouping of pods by zone
func ZonePodsFromContext(ctx context.Context, zone string) []string {
	index := ctx.Value("pod-zone-map").(map[string]string)

	var result []string
	for pod, _zone := range index {
		if _zone == zone {
			result = append(result, pod)
		}
	}

	return result
}

func contextWithAWSConfig(ctx context.Context, config aws.Config) context.Context {
	return context.WithValue(ctx, "aws-config", config)
}

func AWSConfigFromContext(ctx context.Context) aws.Config {
	return ctx.Value("aws-config").(aws.Config)
}
