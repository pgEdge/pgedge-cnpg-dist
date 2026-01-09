package helpers

import (
	"context"
	"fmt"
	"testing"

	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// GetNodes returns the list of nodes in the cluster
func GetNodes(t *testing.T, opts *k8s.KubectlOptions) ([]corev1.Node, error) {
	t.Helper()

	clientset, err := getClientset(opts.ConfigPath)
	if err != nil {
		return nil, err
	}

	nodes, err := clientset.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	return nodes.Items, nil
}

// GetStorageClasses returns the list of storage class names
func GetStorageClasses(t *testing.T, opts *k8s.KubectlOptions) ([]string, error) {
	t.Helper()

	clientset, err := getClientset(opts.ConfigPath)
	if err != nil {
		return nil, err
	}

	storageClasses, err := clientset.StorageV1().StorageClasses().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list storage classes: %w", err)
	}

	names := make([]string, len(storageClasses.Items))
	for i, sc := range storageClasses.Items {
		names[i] = sc.Name
	}

	return names, nil
}

// GetVolumeSnapshotClasses returns the list of volume snapshot class names
func GetVolumeSnapshotClasses(t *testing.T, opts *k8s.KubectlOptions) ([]string, error) {
	t.Helper()

	// Use kubectl to get volume snapshot classes since they require dynamic client
	output, err := k8s.RunKubectlAndGetOutputE(t, opts, "get", "volumesnapshotclasses", "-o", "jsonpath={.items[*].metadata.name}")
	if err != nil {
		return nil, fmt.Errorf("failed to get volume snapshot classes: %w", err)
	}

	if output == "" {
		return []string{}, nil
	}

	// Simple space-separated parsing
	var names []string
	current := ""
	for _, c := range output {
		if c == ' ' {
			if current != "" {
				names = append(names, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		names = append(names, current)
	}

	return names, nil
}

// GetDeployment returns a deployment by name
func GetDeployment(t *testing.T, opts *k8s.KubectlOptions, name string) error {
	t.Helper()

	err := k8s.RunKubectlE(t, opts, "get", "deployment", name)
	if err != nil {
		return fmt.Errorf("failed to get deployment %s: %w", name, err)
	}

	return nil
}

// CRDExists checks if a CRD exists
func CRDExists(t *testing.T, opts *k8s.KubectlOptions, crdName string) (bool, error) {
	t.Helper()

	err := k8s.RunKubectlE(t, opts, "get", "crd", crdName)
	if err != nil {
		// Check if it's a "not found" error
		return false, nil
	}

	return true, nil
}

// getClientset creates a Kubernetes clientset from kubeconfig
func getClientset(kubeconfigPath string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, nil
}

// ApplyManifest applies a Kubernetes manifest from a string
func ApplyManifest(t *testing.T, opts *k8s.KubectlOptions, manifest string) error {
	t.Helper()

	err := k8s.KubectlApplyFromStringE(t, opts, manifest)
	if err != nil {
		return fmt.Errorf("failed to apply manifest: %w", err)
	}

	return nil
}

// DeleteManifest deletes resources from a manifest string
func DeleteManifest(t *testing.T, opts *k8s.KubectlOptions, manifest string) error {
	t.Helper()

	err := k8s.KubectlDeleteFromStringE(t, opts, manifest)
	if err != nil {
		return fmt.Errorf("failed to delete manifest: %w", err)
	}

	return nil
}

// CreateSecret creates a Kubernetes secret
func CreateSecret(t *testing.T, opts *k8s.KubectlOptions, name string, data map[string]string) error {
	t.Helper()

	clientset, err := getClientset(opts.ConfigPath)
	if err != nil {
		return err
	}

	secretData := make(map[string][]byte)
	for k, v := range data {
		secretData[k] = []byte(v)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: opts.Namespace,
		},
		Data: secretData,
	}

	_, err = clientset.CoreV1().Secrets(opts.Namespace).Create(context.Background(), secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}

// WaitForPodsReady waits for a number of pods matching a label selector to be ready
func WaitForPodsReady(t *testing.T, opts *k8s.KubectlOptions, labelSelector string, expectedCount int, retries int) error {
	t.Helper()

	for i := 0; i < retries; i++ {
		clientset, err := getClientset(opts.ConfigPath)
		if err != nil {
			return err
		}

		pods, err := clientset.CoreV1().Pods(opts.Namespace).List(context.Background(), metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err != nil {
			require.NoError(t, err)
		}

		if len(pods.Items) < expectedCount {
			continue
		}

		readyCount := 0
		for _, pod := range pods.Items {
			if isPodReady(&pod) {
				readyCount++
			}
		}

		if readyCount >= expectedCount {
			return nil
		}
	}

	return fmt.Errorf("timeout waiting for %d pods to be ready", expectedCount)
}

// isPodReady checks if a pod is ready
func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
