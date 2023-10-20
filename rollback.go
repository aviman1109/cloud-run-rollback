package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"os/exec"
	"sort"
	"time"
)

type Revision struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Annotations struct {
			AutoscalingKnativeDevMaxScale        string `json:"autoscaling.knative.dev/maxScale"`
			ClientKnativeDevUserImage            string `json:"client.knative.dev/user-image"`
			RunGoogleapisComClientName           string `json:"run.googleapis.com/client-name"`
			RunGoogleapisComCPUThrottling        string `json:"run.googleapis.com/cpu-throttling"`
			RunGoogleapisComExecutionEnvironment string `json:"run.googleapis.com/execution-environment"`
			RunGoogleapisComOperationID          string `json:"run.googleapis.com/operation-id"`
			RunGoogleapisComStartupCPUBoost      string `json:"run.googleapis.com/startup-cpu-boost"`
			ServingKnativeDevCreator             string `json:"serving.knative.dev/creator"`
		} `json:"annotations"`
		CreationTimestamp time.Time `json:"creationTimestamp"`
		Generation        int       `json:"generation"`
		Labels            struct {
			CloudGoogleapisComLocation               string `json:"cloud.googleapis.com/location"`
			ServingKnativeDevConfiguration           string `json:"serving.knative.dev/configuration"`
			ServingKnativeDevConfigurationGeneration string `json:"serving.knative.dev/configurationGeneration"`
			ServingKnativeDevRoute                   string `json:"serving.knative.dev/route"`
			ServingKnativeDevService                 string `json:"serving.knative.dev/service"`
			ServingKnativeDevServiceUID              string `json:"serving.knative.dev/serviceUid"`
		} `json:"labels"`
		Name            string `json:"name"`
		Namespace       string `json:"namespace"`
		OwnerReferences []struct {
			APIVersion         string `json:"apiVersion"`
			BlockOwnerDeletion bool   `json:"blockOwnerDeletion"`
			Controller         bool   `json:"controller"`
			Kind               string `json:"kind"`
			Name               string `json:"name"`
			UID                string `json:"uid"`
		} `json:"ownerReferences"`
		ResourceVersion string `json:"resourceVersion"`
		SelfLink        string `json:"selfLink"`
		UID             string `json:"uid"`
	} `json:"metadata"`
	Spec struct {
		ContainerConcurrency int `json:"containerConcurrency"`
		Containers           []struct {
			Env []struct {
				Name  string `json:"name"`
				Value string `json:"value"`
			} `json:"env"`
			Image string `json:"image"`
			Name  string `json:"name"`
			Ports []struct {
				ContainerPort int    `json:"containerPort"`
				Name          string `json:"name"`
			} `json:"ports"`
			Resources struct {
				Limits struct {
					CPU    string `json:"cpu"`
					Memory string `json:"memory"`
				} `json:"limits"`
			} `json:"resources"`
		} `json:"containers"`
		ServiceAccountName string `json:"serviceAccountName"`
		TimeoutSeconds     int    `json:"timeoutSeconds"`
	} `json:"spec"`
	Status struct {
		Conditions []struct {
			LastTransitionTime time.Time `json:"lastTransitionTime"`
			Reason             string    `json:"reason"`
			Status             string    `json:"status"`
			Type               string    `json:"type"`
			Severity           string    `json:"severity,omitempty"`
		} `json:"conditions"`
		ImageDigest        string `json:"imageDigest"`
		LogURL             string `json:"logUrl"`
		ObservedGeneration int    `json:"observedGeneration"`
	} `json:"status"`
}

func WritePrivateKeyToFile(key string) error {
	file, err := os.Create("/service-account.json")
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(key)
	if err != nil {
		return err
	}
	os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/service-account.json")

	// Execute the gcloud command to activate the service account
	cmd := exec.Command("gcloud", "auth", "activate-service-account", "--key-file=/service-account.json")
	err = cmd.Run()
	if err != nil {
		log.Fatal(err)
	}
	return nil
}
func RetiredRevision(project string, service string, region string) (Revision, error) {
	cmd := exec.Command("gcloud", "run", "revisions", "list", "--project", project, "--service", service, "--region", region, "--format", "json", "--limit", "5")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Executed command: %v\n", cmd.Args)
		log.Printf("Output: %s\n", output)
		return Revision{}, err
	}

	var retiredRevision Revision
	var revisions []Revision
	err = json.Unmarshal(output, &revisions)
	if err != nil {
		return Revision{}, err
	}

	// Sort revisions by creation timestamp, newest first
	sort.Slice(revisions, func(i, j int) bool {
		return revisions[i].Metadata.CreationTimestamp.After(revisions[j].Metadata.CreationTimestamp)
	})

	// Find the first revision that has a "Retired" condition
	for _, r := range revisions {
		for _, c := range r.Status.Conditions {
			if c.Reason == "Retired" && c.Status == "True" {
				retiredRevision = r
				break
			}
		}
		if retiredRevision.Metadata.Name != "" {
			break
		}
	}

	// Check if a retired revision was found
	if retiredRevision.Metadata.Name == "" {
		return Revision{}, errors.New("no retired revision found")
	}

	return retiredRevision, nil
}
func UpdateTraffic(project string, service string, region string, revisionName string) error {
	cmd := exec.Command("gcloud", "run", "services", "update-traffic", service, "--to-revisions", revisionName+"=100", "--project", project, "--region", region)
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Executed command: %v\n", cmd.Args)
		log.Printf("Output: %s\n", output)
		return err
	}
	return nil
}

func main() {
	log.SetFlags(0)
	projectPtr := flag.String("project", "", "the project id of the Cloud Run service to roll back")
	servicePtr := flag.String("service", "", "the name of the Cloud Run service to roll back")
	regionPtr := flag.String("region", "", "the region where the Cloud Run service is deployed")
	privateKey := flag.String("key", "", "the service account credential allow to request Cloud Run service")
	flag.Parse()

	if *privateKey != "" {
		err := WritePrivateKeyToFile(*privateKey)
		if err != nil {
			log.Fatal("Error in WritePrivateKeyToFile():", err)
		}
	}
	if *projectPtr == "" || *servicePtr == "" || *regionPtr == "" {
		log.Fatal("Error: project, service, and region flags are required")
	}

	log.Printf("Rolling back service %s in region %s...\n", *servicePtr, *regionPtr)

	retiredRevision, err := RetiredRevision(*projectPtr, *servicePtr, *regionPtr)
	if err != nil {
		log.Fatal("Error in RetiredRevision():", err)
	}

	log.Printf("Update all traffic to revision %s...\n", retiredRevision.Metadata.Name)
	err = UpdateTraffic(*projectPtr, *servicePtr, *regionPtr, retiredRevision.Metadata.Name)
	if err != nil {
		log.Fatal("Error in UpdateTraffic():", err)
	}
}
