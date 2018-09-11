package main

import (
	"encoding/base64"
	"log"
	"os"
	"strconv"
	"strings"

	sdk "github.com/gaia-pipeline/gosdk"
	vaultapi "github.com/hashicorp/vault/api"
	"k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	kubeConfVaultPath = "secret/data/kube-conf"
	kubeLocalPath     = "/tmp/kubeconfig"
)

var hostDNSName = "host.docker.internal"

// Variables dynamically set during runtime.
var (
	vaultAddress string
	vaultToken   string
	imageName    string
	replicas     int32
	appName      string
	clientSet    *kubernetes.Clientset
)

// GetSecretsFromVault retrieves all information and credentials
// from vault and saves it to local space.
func GetSecretsFromVault(args sdk.Arguments) error {
	// Get vault credentials
	for _, arg := range args {
		switch arg.Key {
		case "vault-token":
			vaultToken = arg.Value
		case "vault-address":
			vaultAddress = arg.Value
		}
	}

	// Create new Vault client instance
	vaultClient, err := vaultapi.NewClient(vaultapi.DefaultConfig())
	if err != nil {
		log.Printf("Error: %s\n", err.Error())
		return err
	}

	// Set vault address
	err = vaultClient.SetAddress(vaultAddress)
	if err != nil {
		log.Printf("Error: %s\n", err.Error())
		return err
	}

	// Set token
	vaultClient.SetToken(vaultToken)

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

	// Create file
	f, err := os.Create(kubeLocalPath)
	if err != nil {
		log.Printf("Error: %s\n", err.Error())
		return err
	}
	defer f.Close()

	// Write to file
	_, err = f.Write(kubeConf)

	log.Println("All data has been retrieved from vault!")
	return nil
}

// PrepareDeployment prepares the deployment by setting up
// the kubernetes client and caching all manual input from user.
func PrepareDeployment(args sdk.Arguments) error {
	// Setup kubernetes client
	config, err := clientcmd.BuildConfigFromFlags("", kubeLocalPath)
	if err != nil {
		return err
	}

	clientSet, err = kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	// Cache given arguments for other jobs
	for _, arg := range args {
		switch arg.Key {
		case "vault-address":
			vaultAddress = arg.Value
		case "image-name":
			imageName = arg.Value
		case "replicas":
			rep, err := strconv.ParseInt(arg.Value, 10, 64)
			if err != nil {
				log.Printf("Error: %s\n", err)
				return err
			}
			replicas = int32(rep)
		case "app-name":
			appName = arg.Value
		}
	}

	return nil
}

// CreateNamespace creates the namespace for our app.
// If the namespace already exists nothing will happen.
func CreateNamespace(args sdk.Arguments) error {
	// Create namespace object
	ns := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: appName,
		},
	}

	// Lookup if namespace already exists
	nsClient := clientSet.Core().Namespaces()
	_, err := nsClient.Get(appName, metav1.GetOptions{})

	// namespace exists
	if err == nil {
		log.Printf("Namespace '%s' already exists. Skipping!", appName)
		return nil
	}

	// Create namespace
	_, err = clientSet.Core().Namespaces().Create(ns)
	if err != nil {
		return err
	}

	log.Printf("Service '%s' has been created!\n", appName)
	return err
}

// CreateDeployment creates the kubernetes deployment.
// If it already exists, it will be updated.
func CreateDeployment(args sdk.Arguments) error {
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
						Image:           imageName,
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
	deployClient := clientSet.ExtensionsV1beta1().Deployments(appName)
	_, err := deployClient.Get(appName, metav1.GetOptions{})

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
func CreateService(args sdk.Arguments) error {
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
	serviceClient := clientSet.Core().Services(appName)
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
			Args: sdk.Arguments{
				sdk.Argument{
					Type: sdk.VaultInp,
					Key:  "vault-token",
				},
				sdk.Argument{
					Type: sdk.VaultInp,
					Key:  "vault-address",
				},
			},
		},
		sdk.Job{
			Handler:     PrepareDeployment,
			Title:       "Prepare Deployment",
			Description: "Prepares the deployment (caches manual input and prepares kubernetes connection)",
			DependsOn:   []string{"Get secrets"},
			Args: sdk.Arguments{
				sdk.Argument{
					Type:        sdk.TextFieldInp,
					Description: "Application name:",
					Key:         "app-name",
					Value:       "myapp",
				},
				sdk.Argument{
					Type:        sdk.TextFieldInp,
					Description: "Full image name including tag:",
					Key:         "image-name",
					Value:       "nginx:1.13",
				},
				sdk.Argument{
					Type:        sdk.TextFieldInp,
					Description: "Number of replicas:",
					Key:         "replicas",
					Value:       "1",
				},
			},
		},
		sdk.Job{
			Handler:     CreateNamespace,
			Title:       "Create namespace",
			Description: "Create kubernetes namespace",
			DependsOn:   []string{"Prepare Deployment"},
		},
		sdk.Job{
			Handler:     CreateDeployment,
			Title:       "Create deployment",
			Description: "Create kubernetes app deployment",
			DependsOn:   []string{"Create namespace"},
		},
		sdk.Job{
			Handler:     CreateService,
			Title:       "Create Service",
			Description: "Create kubernetes service which exposes the service",
			DependsOn:   []string{"Create deployment"},
		},
	}

	// Serve
	if err := sdk.Serve(jobs); err != nil {
		panic(err)
	}
}
