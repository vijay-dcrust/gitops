/*
Copyright 2021 Vijay Pal.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/resourcegroupstaggingapi"

	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	awsv1alpha1 "github.com/gitops/gitops-operator/api/v1alpha1"
)

const (
	// ProviderName is the cloud provider providing loadbalancing functionality
	ProviderName = "aws"
)

// ELB is the struct implementing the lbprovider interface
type ELB struct {
	client            *elb.ELB
	elbResourceName   string
	resourceapiClient *resourcegroupstaggingapi.ResourceGroupsTaggingAPI
}

// NewELB is the factory method for ELB
func NewELB(region string) (*ELB, error) {
	awsConfig := &aws.Config{
		Region: aws.String(region),
		//Credentials: credentials.NewStaticCredentials(id, secret, ""),
		Credentials: credentials.NewSharedCredentials("", "sgcentral"),
	}

	awsConfig = awsConfig.WithCredentialsChainVerboseErrors(true)
	sess, err := session.NewSession(awsConfig)
	if err != nil {
		return nil, fmt.Errorf("Unable to initialize AWS session: %v", err)
	}

	return &ELB{
		client:            elb.New(sess),
		resourceapiClient: resourcegroupstaggingapi.New(sess),
		elbResourceName:   "elasticloadbalancing",
	}, nil
}

// GetLoadbalancersByTag gets the loadbalancers by tag
func (e *ELB) GetLoadbalancersByTag(tagKey string, tagValue string) ([]string, error) {
	tagFilters := &resourcegroupstaggingapi.TagFilter{}
	tagFilters.Key = aws.String(tagKey)
	tagFilters.Values = append(tagFilters.Values, aws.String(tagValue))

	getResourcesInput := &resourcegroupstaggingapi.GetResourcesInput{}
	getResourcesInput.TagFilters = append(getResourcesInput.TagFilters, tagFilters)
	getResourcesInput.ResourceTypeFilters = append(
		getResourcesInput.ResourceTypeFilters,
		aws.String(e.elbResourceName),
	)

	resources, err := e.resourceapiClient.GetResources(getResourcesInput)
	if err != nil {
		return nil, err
	}

	elbs := []string{}
	for _, resource := range resources.ResourceTagMappingList {
		//fmt.Printf("%+v\n", *resource)

		elbs = append(elbs, *resource.ResourceARN)
	}
	return elbs, nil
}

// GitagentReconciler reconciles a Gitagent object
type GitagentReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=aws.gitops.com,resources=gitagents,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=aws.gitops.com,resources=gitagents/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=aws.gitops.com,resources=gitagents/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Gitagent object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *GitagentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = r.Log.WithValues("gitagent", req.NamespacedName)

	gitAgent := &awsv1alpha1.Gitagent{}
	err := r.Get(ctx, req.NamespacedName, gitAgent)
	if err != nil {
		if errors.IsNotFound(err) {
			println("GitAgent resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
	}

	elbTags := gitAgent.Spec.ServiceTags

	i := 0
	tagKeys := make([]string, len(elbTags))
	tagValues := make([]string, len(elbTags))
	for k, v := range elbTags {
		tagKeys[i] = k
		tagValues[i] = v
		i++
	}
	awsConfig := &aws.Config{
		Region: aws.String("ap-southeast-1"),
		//Credentials: credentials.NewStaticCredentials(id, secret, ""),
		Credentials: credentials.NewSharedCredentials("", "sgcentral"),
	}
	awsConfig = awsConfig.WithCredentialsChainVerboseErrors(true)

	svc := elb.New(session.New(awsConfig))
	input := &elb.DescribeLoadBalancersInput{}
	result, err := svc.DescribeLoadBalancers(input)
	if err != nil {
		fmt.Println(err.Error())
	}

	lb_output := result.LoadBalancerDescriptions[0]
	lb_name := *(lb_output.LoadBalancerName)

	var list_of_lbname []*string
	list_of_lbname = append(list_of_lbname, &lb_name)

	input2 := &elb.DescribeTagsInput{
		LoadBalancerNames: list_of_lbname,
	}
	lb_tags, _ := svc.DescribeTags(input2)
	fmt.Println("lb_tags: ", *lb_tags.TagDescriptions[0].Tags[0].Key)

	var matchFound bool
	for _, rt := range lb_tags.TagDescriptions[0].Tags {
		if *(rt.Key) == tagKeys[0] && *(rt.Value) == tagValues[0] {
			print("key found")
			matchFound = true
			gitAgent.Status.AwsComponentName = "Matching found"
			break
		}
	}
	if !matchFound {
		gitAgent.Status.AwsComponentName = "Matching Not found"
	}
	err = r.Status().Update(ctx, gitAgent)
	if err != nil {
		println(err, "Failed to update Memcached status")
		return ctrl.Result{}, err
	}

	//fmt.Println("result: ", lb_dns_name.AvailabilityZones)
	return ctrl.Result{Requeue: true}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitagentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.Gitagent{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
