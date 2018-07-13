package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"strings"

	sdk "github.com/gaia-pipeline/gosdk"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	vaultAddress        = "http://vault:8200"
	vaultToken          = "root-token"
	kubeConfVaultPath   = "secret/data/kube-conf"
	appVersionVaultPath = "secret/data/nginx"
	kubeLocalPath       = "/tmp/kube-conf"
	appVersionLocalPath = "/tmp/app-version"
	hostDNSName         = "host.docker.internal"

	// Deployment specific attributes
	appName        = "nginx"
	replicas int32 = 2
)

// GetSecretsFromVault retrieves all information and credentials
// from vault and saves it to local space.
func GetSecretsFromVault() error {
	// Create vault client
	vaultClient, err := connectToVault()
	if err != nil {
		log.Printf("Error: %s\n", err.Error())
		return err
	}

	// Read kube config from vault and decode base64
	l := vaultClient.Logical()
	s, err := l.Read(kubeConfVaultPath)
	if err != nil {
		log.Printf("Error: %s\n", err.Error())
		return err
	}
	conf := s.Data["data"].(map[string]interface{})
	kubeConf, err := base64.StdEncoding.DecodeString(conf["conf"].(string))
	if err != nil {
		log.Printf("Error: %s\n", err.Error())
		return err
	}

	// Convert config to string and replace localhost.
	// We use here the magical DNS name "host.docker.internal",
	// which resolves to the internal IP address used by the host.
	// If this should not work for you, replace it with your real IP address.
	confStr := string(kubeConf[:])
	confStr = strings.Replace(confStr, "localhost", hostDNSName, 1)
	kubeConf = []byte(confStr)

	// Write kube config to file
	if err = writeToFile(kubeLocalPath, kubeConf); err != nil {
		log.Printf("Error: %s\n", err.Error())
		return err
	}

	// Read app image version from vault
	v, err := l.Read(appVersionVaultPath)
	if err != nil {
		log.Printf("Error: %s\n", err.Error())
		return err
	}

	// Write image version to file
	version := (v.Data["data"].(map[string]interface{}))["version"].(string)
	if err = writeToFile(appVersionLocalPath, []byte(version)); err != nil {
		log.Printf("Error: %s\n", err.Error())
		return err
	}
	log.Println("All data has been retrieved from vault!")
	return nil
}

// CreateNamespace creates the namespace for our app.
// If the namespace already exists nothing will happen.
func CreateNamespace() error {
	// Get kubernetes client
	c, err := getKubeClient(kubeLocalPath)
	if err != nil {
		return err
	}

	// Create namespace object
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: appName,
		},
	}

	// Lookup if namespace already exists
	nsClient := c.Core().Namespaces()
	_, err = nsClient.Get(appName, metav1.GetOptions{})

	// namespace exists
	if err == nil {
		log.Printf("Namespace '%s' already exists. Skipping!", appName)
		return nil
	}

	// Create namespace
	_, err = c.Core().Namespaces().Create(ns)
	if err != nil {
		return err
	}

	log.Printf("Service '%s' has been created!\n", appName)
	return err
}

// CreateDeployment creates the kubernetes deployment.
// If it already exists, it will be updated.
func CreateDeployment() error {
	// Get kubernetes client
	c, err := getKubeClient(kubeLocalPath)
	if err != nil {
		log.Printf("Error: %s\n", err.Error())
		return err
	}

	// Load image version from file
	v, err := ioutil.ReadFile(appVersionLocalPath)
	if err != nil {
		log.Printf("Error: %s\n", err.Error())
		return err
	}

	// Create deployment object
	d := &v1beta1.Deployment{}
	d.ObjectMeta = metav1.ObjectMeta{
		Name: appName,
		Labels: map[string]string{
			"app": appName,
		},
	}
	d.Spec = v1beta1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": appName,
			},
		},
		Template: v1.PodTemplateSpec{
			ObjectMeta: d.ObjectMeta,
			Spec: v1.PodSpec{
				Containers: []v1.Container{
					v1.Container{
						Name:            appName,
						Image:           fmt.Sprintf("%s:%s", appName, v),
						ImagePullPolicy: v1.PullAlways,
						Ports: []v1.ContainerPort{
							v1.ContainerPort{
								ContainerPort: int32(80),
							},
						},
					},
				},
			},
		},
	}

	// Lookup existing deployments
	deployClient := c.ExtensionsV1beta1().Deployments(appName)
	_, err = deployClient.Get(appName, metav1.GetOptions{})

	// Deployment already exists
	if err == nil {
		_, err = deployClient.Update(d)
		if err != nil {
			log.Printf("Error: %s\n", err.Error())
			return err
		}
		log.Printf("Deployment '%s' has been updated!\n", appName)
		return nil
	}

	// Create deployment object in kubernetes
	_, err = deployClient.Create(d)
	if err != nil {
		log.Printf("Error: %s\n", err.Error())
		return err
	}
	log.Printf("Deployment '%s' has been created!\n", appName)
	return nil
}

// CreateService creates the service for our application.
// If the service already exists, it will be updated.
func CreateService() error {
	// Get kubernetes client
	c, err := getKubeClient(kubeLocalPath)
	if err != nil {
		log.Printf("Error: %s\n", err.Error())
		return err
	}

	// Create service obj
	s := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name: appName,
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"app": appName,
			},
			Type: v1.ServiceTypeNodePort,
			Ports: []v1.ServicePort{
				v1.ServicePort{
					Protocol:   v1.ProtocolTCP,
					Port:       int32(8090),
					TargetPort: intstr.FromInt(80),
				},
			},
		},
	}

	// Lookup for existing service
	serviceClient := c.Core().Services(appName)
	currService, err := serviceClient.Get(appName, metav1.GetOptions{})

	// Service already exists
	if err == nil {
		s.ObjectMeta = currService.ObjectMeta
		s.Spec.ClusterIP = currService.Spec.ClusterIP
		_, err = serviceClient.Update(s)
		if err != nil {
			log.Printf("Error: %s\n", err.Error())
			return err
		}
		log.Printf("Service '%s' has been updated!\n", appName)
		return nil
	}

	// Create service
	_, err = serviceClient.Create(s)
	if err != nil {
		log.Printf("Error: %s\n", err.Error())
		return err
	}
	log.Printf("Service '%s' has been created!\n", appName)
	return nil
}

func main() {
	jobs := sdk.Jobs{
		sdk.Job{
			Handler:     GetSecretsFromVault,
			Title:       "Get secrets",
			Description: "Get secrets from vault",
			Priority:    0,
		},
		sdk.Job{
			Handler:     CreateNamespace,
			Title:       "Create namespace",
			Description: "Create kubernetes namespace",
			Priority:    10,
		},
		sdk.Job{
			Handler:     CreateDeployment,
			Title:       "Create deployment",
			Description: "Create kubernetes app deployment",
			Priority:    20,
		},
		sdk.Job{
			Handler:     CreateService,
			Title:       "Create Service",
			Description: "Create kubernetes service which exposes the service",
			Priority:    30,
		},
	}

	// Serve
	if err := sdk.Serve(jobs); err != nil {
		panic(err)
	}
}
