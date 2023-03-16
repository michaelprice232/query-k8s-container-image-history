# query-k8s-container-image-history

Queries for keywords that are present in the Docker image history of all the pods that are running in a K8s cluster. Can be useful to help identify hidden dependencies in images such as older Java runtime versions.

Performs the following tasks:

- Pulls ECR credentials for all regions configured via variable `ecrRegions` ready for Docker login using a profile
- Queries all the pods running in the cluster and dedups the container images
- Pulls each image locally and inspects the history of the image for keywords. Requires Docker to be running locally
- Matching images are written to a local file: `offending-images-<k8s-context>-<date>.txt`
- Any images which are not ECR based (Dockerhub etc.) are written to a local file: `non-ecr-images-<k8s-context>-<date>.txt`
- Clears the image from the local cache

## Parameters

- `clusterK8sContextName` - the context name in the `${HOME}/.kube/config` file which you want to check the container image history against. All pods will be queried in this cluster
- `imagesAccountAWSProfileName` - AWS profile name in the `${HOME}/.aws/config` file which you want to use to generate ECR credentials to enable Docker login. Should target a profile with permissions to the image's ECR registries
- `dockerImageKeyWords` - comma separated list of keywords to search for in each history layer of each container image

## Running
```shell
% go run main.go --clusterK8sContextName "prod-cluster" --imagesAccountAWSProfileName "prod-account" --dockerImageKeyWords "openjdk-8,openjdk8,jdk-14,jdk14"
```
