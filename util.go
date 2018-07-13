package main

import (
	"os"

	vaultapi "github.com/hashicorp/vault/api"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// getKubeClient creates a new kubernetes client with the given kube config.
func getKubeClient(configPath string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

// writeToFile writes bytes to local file.
func writeToFile(path string, content []byte) error {
	// Create file
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write to file
	_, err = f.Write(content)
	return err
}

// connectToVault creates a new client for accessing vault.
func connectToVault() (*vaultapi.Client, error) {
	// Create new Vault client instance
	vaultClient, err := vaultapi.NewClient(vaultapi.DefaultConfig())
	if err != nil {
		return nil, err
	}

	// Set vault address
	err = vaultClient.SetAddress(vaultAddress)
	if err != nil {
		return nil, err
	}

	// Set token
	vaultClient.SetToken(vaultToken)

	return vaultClient, nil
}
