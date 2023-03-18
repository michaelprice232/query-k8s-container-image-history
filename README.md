# query-k8s-container-image-history

Queries for keywords that are present in the Docker image history (Docker build layer) of all the pods that are running in a K8s cluster. Can be useful to help identify hidden dependencies in images such as older Java runtime versions.

If an image is stored in a private AWS ECR registry then it attempts to authenticate using credentials generated from the AWS ECR client (ecrRegions flag must be set to enable this).

Writes the results to two files:
1) Contains a list of images running in the cluster which have a history matching at least 1 keyword
2) Contains a list of images running in the cluster which are NOT running in private ECR registries (e.g. Dockerhub)

Performs the following tasks:

- Generates ECR credentials using the AWS profile for all regions configured via the `ecrRegions` flag ready for image pulling
- Queries all the pods running in the cluster and dedups the container images
- Pulls each image locally and inspects the history of the image for keywords. Requires Docker to be running locally
- Matching images are written to a local file: `offending-images-<k8s-context>-<date>.txt`
- Any images which are not ECR based (Dockerhub etc.) are written to a local file: `non-ecr-images-<k8s-context>-<date>.txt`
- Clears the images from the local cache

## Pre-reqs
- Docker is running locally
- AWS profile is configured in `${HOME}/.aws/config`, with a principle which has IAM permissions to generate ECR auth tokens and pull images
- K8s context is configured in `${HOME}/.kube/config`, with a user which has RBAC permissions to list and read from all pods
- Go installed: `v1.18+`

## Parameters
- `clusterK8sContextName` - the context name in the `${HOME}/.kube/config` file which you want to check all the container image histories against. All pods/containers will be queried in this cluster
- `imagesAccountAWSProfileName` - AWS profile name in the `${HOME}/.aws/config` file which you want to use to generate ECR credentials to enable Docker login. Should target a profile with permissions to the image's ECR registries
- `ecrRegions` - (optional) comma separate list of AWS regions which contain private ECR registries for running images. Creates a Docker auth token for each via the ECR endpoints
- `dockerImageKeyWords` - comma separated list of keywords to search for in each history layer of each container image

## Running
```shell
# Ensure pre-req's are met above. ecrRegions is optional and required only if you have private ECR based images
% go run ./cmd/main.go --clusterK8sContextName "prod-cluster" --imagesAccountAWSProfileName "production" --dockerImageKeyWords "openjdk-8,openjdk8,jdk-14,jdk14" --ecrRegions "eu-west-1,eu-west-2"
```
