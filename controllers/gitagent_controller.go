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
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"

	"github.com/go-logr/logr"

	"context"

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

func parseTagKeyValues(gitAgent *awsv1alpha1.Gitagent) (tk []string, tv []string) {
	i := 0
	elbTags := gitAgent.Spec.ServiceTags
	tagKeys := make([]string, len(elbTags))
	tagValues := make([]string, len(elbTags))
	for k, v := range elbTags {
		tagKeys[i] = k
		tagValues[i] = v
		i++
	}
	return tagKeys, tagValues
}

func GetAuth(region string) *aws.Config {
	awsConfig := &aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewSharedCredentials("", "sgdev"),
	}
	awsConfig = awsConfig.WithCredentialsChainVerboseErrors(true)
	return awsConfig
}
func GetLBNameList() (service *elb.ELB, lbname []string) {
	awsConfig := GetAuth("ap-southeast-1")
	svc := elb.New(session.New(awsConfig))
	input := &elb.DescribeLoadBalancersInput{}
	result, err := svc.DescribeLoadBalancers(input)
	if err != nil {
		log.Fatalf(err.Error())
	}
	var lb_list []string
	for _, ln := range result.LoadBalancerDescriptions {
		lb_list = append(lb_list, *(ln.LoadBalancerName))
	}
	log.Println("Total Number of load balancers: ", len(result.LoadBalancerDescriptions))
	log.Println("Name of load balancers: ", lb_list)
	return svc, lb_list

}

func MatchLBTags(tagKeys []string, tagValues []string) (lb_name string) {
	svc, lb_list := GetLBNameList()
	var list_of_lbname []*string
	isFound := false
	for _, lb_name := range lb_list {
		list_of_lbname = append(list_of_lbname, &lb_name)

		input := &elb.DescribeTagsInput{
			LoadBalancerNames: list_of_lbname,
		}
		lb_tags, _ := svc.DescribeTags(input)
		log.Println("Number of tags on load balancer: ", lb_name, len(lb_tags.TagDescriptions[0].Tags))

		for i := 0; i < len(tagKeys); i++ {
			for _, rt := range lb_tags.TagDescriptions[0].Tags {
				if *(rt.Key) == tagKeys[i] && *(rt.Value) == tagValues[i] {
					isFound = true
					break
				} else {
					isFound = false
				}
			}
		}
		if isFound {
			return lb_name
		}
	}
	return "Not found"
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
	log := r.Log.WithValues("gitagent", req.NamespacedName)
	gitAgent := &awsv1alpha1.Gitagent{}
	err := r.Get(ctx, req.NamespacedName, gitAgent)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Error(err, "GitAgent resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
	}

	tagKeys, tagValues := parseTagKeyValues(gitAgent)

	lbName := MatchLBTags(tagKeys, tagValues)

	gitAgent.Status.AwsComponentName = lbName
	err = r.Status().Update(ctx, gitAgent)
	if err != nil {
		log.Error(err, "Failed to update GitAgent status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitagentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.Gitagent{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		Complete(r)
}
