package main

import (
	"flag"
	"log"
	"strings"

	"query-k8s-container-image-history/internal/docker-image-history"
)

var (
	clusterK8sContextName       string
	imagesAccountAWSProfileName string
	dockerImageKeyWordsFlag     string
	dockerImageKeyWords         []string
	ecrRegionsFlag              string
	ecrRegions                  []string
)

func main() {
	parseFlags()
	log.Printf("Using K8s Context: '%s'", clusterK8sContextName)
	log.Printf("Using AWS Profile '%s' to pull ECR permissions for the regions: %v", imagesAccountAWSProfileName, ecrRegions)
	log.Printf("Searching for these keywords in image history of all pods in cluster: %v", dockerImageKeyWords)

	cfg, err := docker_image_history.NewConfig(dockerImageKeyWords, clusterK8sContextName, imagesAccountAWSProfileName, ecrRegions)
	if err != nil {
		log.Fatalf("loading config: %s", err)
	}

	if err = cfg.ProcessAllImagesHistoryForKeywords(); err != nil {
		log.Fatalln(err)
	}
}

// parseFlags parses the CLI flags passed
func parseFlags() {
	flag.StringVar(&clusterK8sContextName, "clusterK8sContextName", "", "Context to use in K8s config file in ${HOME}/.kube/config")
	flag.StringVar(&imagesAccountAWSProfileName, "imagesAccountAWSProfileName", "", "AWS profile name to use to authenticate for pulling ECR based Docker images")
	flag.StringVar(&dockerImageKeyWordsFlag, "dockerImageKeyWords", "", "Comma separated list of keywords to search for in image history of K8s pods running in the cluster")
	flag.StringVar(&ecrRegionsFlag, "ecrRegions", "", "Optional: Comma separated list of AWS regions which private ECR registries are present in. Auth tokens will be generated for each")
	flag.Parse()

	if len(dockerImageKeyWordsFlag) > 0 {
		dockerImageKeyWords = strings.Split(dockerImageKeyWordsFlag, ",")
	}
	if len(clusterK8sContextName) == 0 || len(imagesAccountAWSProfileName) == 0 || len(dockerImageKeyWords) == 0 {
		log.Fatalln("Usage: query-k8s-container-image-history -clusterK8sContextName=<context> -imagesAccountAWSProfileName=<profile> -dockerImageKeyWords='keyword1,keyword2'")
	}
	if len(ecrRegionsFlag) > 0 {
		ecrRegions = strings.Split(ecrRegionsFlag, ",")
		if !docker_image_history.ValidateAWSRegions(ecrRegions) {
			log.Fatalf("One or more parsed AWS regions are invalid: %v, Allowed regions: %v", ecrRegions, docker_image_history.AllAWSRegions)
		}
	} else {
		log.Println("No AWS regions have been configured via the ecrRegions flag. Only public registries will be allowed")
	}
}
