package docker_image_history

import (
	dockerClient "github.com/docker/docker/client"
	"k8s.io/client-go/kubernetes"
)

// Config stores the Docker & K8s clients as well as the results from searching for keywords in image history
type Config struct {
	dockerImageKeyWords         []string
	dockerImages                map[string][]podDetails
	offendingDockerImages       []offendingDockerImage
	dockerClient                *dockerClient.Client
	ecrCredentials              map[string]string
	ecrRegions                  []string
	k8sClient                   *kubernetes.Clientset
	clusterK8sContextName       string
	imagesAccountAWSProfileName string
}

// podDetails provides K8s context for any images which have been matched in the cluster
type podDetails struct {
	podName       string
	containerName string
	namespace     string
}

// offendingDockerImage stores a result of an image which has been matched against the target keywords
type offendingDockerImage struct {
	matchFound      bool
	imageRef        string
	matchedKeywords map[string]int
}

// Event stores the data parsed from each Docker image pull log
type Event struct {
	Status         string `json:"status"`
	Error          string `json:"error"`
	Progress       string `json:"progress"`
	ProgressDetail struct {
		Current int `json:"current"`
		Total   int `json:"total"`
	} `json:"progressDetail"`
}
