package main

import (
	"fmt"
	"os"
	"testing"

	sdk "github.com/gaia-pipeline/gosdk"
)

func TestMain(m *testing.M) {
	hostDNSName = "localhost"
	err := GetSecretsFromVault(sdk.Arguments{})
	if err != nil {
		fmt.Printf("Cannot retrieve data from vault: %s\n", err.Error())
		os.Exit(1)
	}

	err = PrepareDeployment(createTestArguments())
	if err != nil {
		fmt.Printf("Cannot prepare the deployment: %s\n", err.Error())
		os.Exit(1)
	}

	err = CreateNamespace(sdk.Arguments{})
	if err != nil {
		fmt.Printf("Cannot create namespace: %s\n", err.Error())
		os.Exit(1)
	}

	r := m.Run()
	os.Exit(r)
}

func TestCreateService(t *testing.T) {
	err := CreateService(sdk.Arguments{})
	if err != nil {
		t.Error(err)
	}
}

func TestCreateDeployment(t *testing.T) {
	err := CreateDeployment(sdk.Arguments{})
	if err != nil {
		t.Error(err)
	}
}

func createTestArguments() sdk.Arguments {
	return sdk.Arguments{
		sdk.Argument{
			Type:  sdk.VaultInp,
			Key:   "vault-token",
			Value: "123456789",
		},
		sdk.Argument{
			Type:  sdk.VaultInp,
			Key:   "vault-address",
			Value: "http://localhost:8200",
		},
		sdk.Argument{
			Type:  sdk.TextFieldInp,
			Key:   "image-name",
			Value: "nginx:1.13",
		},
		sdk.Argument{
			Type:  sdk.TextFieldInp,
			Key:   "app-name",
			Value: "nginx",
		},
		sdk.Argument{
			Type:  sdk.TextFieldInp,
			Key:   "replicas",
			Value: "1",
		},
	}
}
