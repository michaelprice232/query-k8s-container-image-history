package docker_image_history

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/docker/docker/api/types"
	dockerClient "github.com/docker/docker/client"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

var AllAWSRegions = []string{"af-south-1", "ap-south-1", "eu-north-1", "eu-west-3", "eu-west-2", "eu-west-1", "ap-northeast-3", "ap-northeast-2",
	"ap-northeast-1", "ca-central-1", "sa-east-1", "ap-southeast-1", "ap-southeast-2", "eu-central-1", "us-east-1", "us-east-2", "us-west-1",
	"us-west-2",
}

// ProcessAllImagesHistoryForKeywords queries all images of containers running in the cluster and checks their history to see if it matches 1 or more keywords
// Writes results to 2 files:
// 1) Images which have a history containing at least 1 keyword
// 2) Images which are not stored in an AWS ECR registry
func (c *Config) ProcessAllImagesHistoryForKeywords() error {

	defer func(dockerClient *dockerClient.Client) {
		err := dockerClient.Close()
		if err != nil {
			log.Printf("closing Docker client: %s", err)
		}
	}(c.dockerClient)

	if err := c.queryAllContainerImageRefsInCluster(); err != nil {
		return err
	}

	totalUniqueImages := len(c.dockerImages)
	count := 1
	for image := range c.dockerImages {
		fmt.Printf("Pulling image (%d / %d): %s\n", count, totalUniqueImages, image)
		err := c.pullImage(image)
		if err != nil {
			return err
		}

		count++

		result, err := c.checkImageHistoryForKeyWords(image)
		if err != nil {
			return err
		}
		if result.matchFound {
			c.offendingDockerImages = append(c.offendingDockerImages, result)
		}

		if err = c.cleanupImage(image); err != nil {
			return err
		}
	}

	err := c.outputOffendingImages()
	if err != nil {
		return err
	}

	err = c.outputNonECRImages()
	if err != nil {
		return err
	}

	return nil
}

// NewConfig returns a new Config with initialised Docker & K8s clients
func NewConfig(keywords []string, clusterAccountProfile, imagesAccountProfile string, ecrRegions []string) (*Config, error) {
	cfg := &Config{}

	cfg.imagesAccountAWSProfileName = imagesAccountProfile
	cfg.clusterK8sContextName = clusterAccountProfile
	cfg.dockerImageKeyWords = keywords
	cfg.dockerImages = make(map[string][]podDetails)
	cfg.offendingDockerImages = make([]offendingDockerImage, 0)
	cfg.ecrCredentials = make(map[string]string)
	cfg.ecrRegions = ecrRegions

	// Get Docker login credentials via ECR API for each AWS region images are present in
	for _, region := range cfg.ecrRegions {
		awsConfig, err := config.LoadDefaultConfig(context.Background(), config.WithSharedConfigProfile(imagesAccountProfile), config.WithRegion(region))
		if err != nil {
			return nil, fmt.Errorf("loading AWS config: %s", err)
		}
		ecrClient := ecr.NewFromConfig(awsConfig)

		ecrResp, err := ecrClient.GetAuthorizationToken(context.Background(), &ecr.GetAuthorizationTokenInput{})
		if err != nil {
			return nil, fmt.Errorf("getting ECR auth token: %s", err)
		}

		decodedToken, err := base64.StdEncoding.DecodeString(*ecrResp.AuthorizationData[0].AuthorizationToken)
		if err != nil {
			return nil, fmt.Errorf("decoding ECR auth token: %s", err)
		}
		credentialsSlice := strings.Split(string(decodedToken), ":")
		jsonBytes, err := json.Marshal(map[string]string{"username": "AWS", "password": credentialsSlice[1]})
		if err != nil {
			return nil, fmt.Errorf("marshalling ECR creds into JSON: %s", err)
		}
		cfg.ecrCredentials[region] = base64.StdEncoding.EncodeToString(jsonBytes)
	}

	// Docker client
	dockerCli, err := dockerClient.NewClientWithOpts(dockerClient.FromEnv, dockerClient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("creating Docker client: %s", err)
	}
	cfg.dockerClient = dockerCli

	// K8s client
	k8sConfig, err := buildConfigWithContextFromFlags(clusterAccountProfile, filepath.Join(homedir.HomeDir(), ".kube", "config"))
	if err != nil {
		return nil, fmt.Errorf("loading k8s config file: %s", err)
	}
	k8ClientSet, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		return nil, fmt.Errorf("creating k8s client set: %s", err)
	}
	cfg.k8sClient = k8ClientSet

	return cfg, nil
}

// buildConfigWithContextFromFlags returns a k8s client config which has overridden the context
func buildConfigWithContextFromFlags(context string, kubeconfigPath string) (*rest.Config, error) {
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath},
		&clientcmd.ConfigOverrides{
			CurrentContext: context,
		}).ClientConfig()
}

// outputNonECRImages writes to a file all the container images in the cluster which are not stored in an AWS ECR registry
func (c *Config) outputNonECRImages() error {
	nonECRImageResultsPath := fmt.Sprintf("non-ecr-images-%s-%s.txt", c.clusterK8sContextName, time.Now().Format("2-Jan-2006-15:04"))

	f, err := os.OpenFile(nonECRImageResultsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening file '%s': %s", nonECRImageResultsPath, err)
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Printf("problem closing file '%s': %s", nonECRImageResultsPath, err)
		}
	}(f)

	for image, details := range c.dockerImages {
		if !strings.Contains(image, "amazonaws.com") {
			_, err := f.WriteString(fmt.Sprintf("%s\t", image))
			for _, match := range details {
				_, err = f.WriteString(fmt.Sprintf("(podName: %s, containerName: %s, namespace: %s) ", match.podName, match.containerName, match.namespace))
			}
			_, err = f.WriteString("\n")

			if err != nil {
				return fmt.Errorf("writing results to '%s': %s", nonECRImageResultsPath, err)
			}
		}
	}
	log.Printf("Non ECR based image results written to: %s", nonECRImageResultsPath)

	return nil
}

// outputOffendingImages writes to a file all the container images in the cluster which have a history which have matched 1 or more keywords
func (c *Config) outputOffendingImages() error {
	offendingImageResultsPath := fmt.Sprintf("offending-images-%s-%s.txt", c.clusterK8sContextName, time.Now().Format("2-Jan-2006-15:04"))

	if len(c.offendingDockerImages) > 0 {
		f, err := os.OpenFile(offendingImageResultsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("opening file '%s': %s", offendingImageResultsPath, err)
		}
		defer func(f *os.File) {
			err := f.Close()
			if err != nil {
				log.Printf("problem closing file '%s': %s", offendingImageResultsPath, err)
			}
		}(f)

		for _, i := range c.offendingDockerImages {
			details := c.dockerImages[i.imageRef]
			_, err = f.WriteString(fmt.Sprintf("%s\t", i.imageRef))
			for _, match := range details {
				_, err = f.WriteString(fmt.Sprintf("(podName: %s, containerName: %s, namespace: %s, matched-keywords: %v) ", match.podName, match.containerName, match.namespace, i.matchedKeywords))
			}
			_, err = f.WriteString("\n")
			if err != nil {
				return fmt.Errorf("writing results to '%s': %s", offendingImageResultsPath, err)
			}
		}
		log.Printf("Offending image results written to: %s", offendingImageResultsPath)

	} else {
		fmt.Println("No images matched keywords. Nothing to output.")
	}

	return nil
}

// queryAllContainerImageRefsInCluster queries for all the containers running as pods in the cluster and stores them in the Config for later processing
func (c *Config) queryAllContainerImageRefsInCluster() error {
	pods, err := c.k8sClient.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("querying for all k8s pods: %s", err)
	}
	log.Printf("Number of pods discovered in cluster: %d\n", len(pods.Items))

	for _, pod := range pods.Items {
		for _, container := range pod.Spec.Containers {
			pd := podDetails{
				podName:       pod.Name,
				containerName: container.Name,
				namespace:     pod.Namespace,
			}
			c.dockerImages[container.Image] = append(c.dockerImages[container.Image], pd)
		}
	}
	log.Printf("Number of unique container image refs: %d", len(c.dockerImages))

	return nil
}

// checkImageHistoryForKeyWords checks the history single Docker image for a set of keywords
// Returns offendingDockerImage which includes whether a match has been found, and details of the matches if so
func (c *Config) checkImageHistoryForKeyWords(imageRef string) (offendingDockerImage, error) {
	var result offendingDockerImage
	result.matchedKeywords = make(map[string]int)

	history, err := c.dockerClient.ImageHistory(context.Background(), imageRef)
	if err != nil {
		return result, fmt.Errorf("querying image history for '%s': %s", imageRef, err)
	}

	for _, h := range history {
		for _, keyword := range c.dockerImageKeyWords {
			if strings.Contains(strings.ToLower(h.CreatedBy), strings.ToLower(keyword)) {
				result.matchFound = true
				result.imageRef = imageRef
				result.matchedKeywords[keyword]++
				fmt.Printf("FOUND: %+v\n", result)
			}
		}
	}
	return result, nil
}

// pullImage pulls a single Docker image using the local Docker instance. Credentials are passed if it's an ECR registry
func (c *Config) pullImage(imageReference string) error {
	// Only pass the Docker credentials if it's an ECR registry. Credentials differ per AWS region
	var pullOptions types.ImagePullOptions

	if strings.Contains(imageReference, "amazonaws.com") {
		foundRegion := false
		for _, region := range c.ecrRegions {
			if strings.Contains(imageReference, fmt.Sprintf("dkr.ecr.%s.amazonaws.com", region)) {
				pullOptions.RegistryAuth = c.ecrCredentials[region]
				foundRegion = true
			}
		}
		if !foundRegion {
			return fmt.Errorf("unsupported ECR image region detected. Currently supported: %v", c.ecrRegions)
		}
	}

	events, err := c.dockerClient.ImagePull(context.Background(), imageReference, pullOptions)
	if err != nil {
		return fmt.Errorf("pulling image '%s': %s", imageReference, err)
	}

	d := json.NewDecoder(events)
	var event *Event
	timeout := time.Now().Add(time.Minute * 10) // cancel stalled downloads
	for {
		if time.Now().After(timeout) {
			return fmt.Errorf("timed out (10 minutes) whilst attempting to download %s", imageReference)
		}

		if err := d.Decode(&event); err != nil {
			if err != io.EOF {
				return fmt.Errorf("decoding Docker image pull JSON output: %s", err)
			}
		}

		// wait until the image is downloaded
		if strings.Contains(event.Status, "Downloaded newer image") || strings.Contains(event.Status, "Image is up to date") {
			return nil
		}
	}
}

// cleanupImage removes a single Docker image from the local cache
func (c *Config) cleanupImage(imageReference string) error {
	_, err := c.dockerClient.ImageRemove(context.Background(), imageReference, types.ImageRemoveOptions{Force: true, PruneChildren: true})
	if err != nil {
		return fmt.Errorf("cleaning up local image '%s': %s", imageReference, err)
	}
	return nil
}

// ValidateAWSRegions validates whether all the regions are valid AWS region codes
func ValidateAWSRegions(regions []string) bool {
	for _, r := range regions {
		if !sliceContains(AllAWSRegions, r) {
			return false
		}
	}
	return true
}

// sliceContains returns whether s is in the string slice
func sliceContains(slice []string, s string) bool {
	found := false
	for _, e := range slice {
		if e == s {
			found = true
		}
	}
	return found
}
