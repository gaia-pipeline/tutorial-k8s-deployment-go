package main

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	vaultAddress = "http://localhost:8200"
	hostDNSName = "localhost"
	err := GetSecretsFromVault()
	if err != nil {
		fmt.Printf("Cannot retrieve data from vault: %s\n", err.Error())
		os.Exit(1)
	}

	err = CreateNamespace()
	if err != nil {
		fmt.Printf("Cannot create namespace: %s\n", err.Error())
		os.Exit(1)
	}

	r := m.Run()
	os.Exit(r)
}

func TestCreateService(t *testing.T) {
	err := CreateService()
	if err != nil {
		t.Error(err)
	}
}

func TestCreateDeployment(t *testing.T) {
	err := CreateDeployment()
	if err != nil {
		t.Error(err)
	}
}
