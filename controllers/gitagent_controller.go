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
	"log"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/go-logr/logr"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"

	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	awsv1alpha1 "github.com/gitops/gitops-operator/api/v1alpha1"
	util "github.com/gitops/gitops-operator/helpers/utilities"
)

const (
	// ProviderName is the cloud provider providing loadbalancing functionality
	ProviderName = "aws"
)

func GetAuth(region string) *aws.Config {
	awsConfig := &aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewSharedCredentials("", "sgdev"),
	}
	awsConfig = awsConfig.WithCredentialsChainVerboseErrors(true)
	return awsConfig
}
func GetLBNameList() (service *elb.ELB, lbname []string, dnsList []string) {
	awsConfig := GetAuth("ap-southeast-1")
	svc := elb.New(session.New(awsConfig))
	input := &elb.DescribeLoadBalancersInput{}
	result, err := svc.DescribeLoadBalancers(input)
	if err != nil {
		log.Fatalf(err.Error())
	}
	var lbList []string
	var lbDnsList []string
	for _, ln := range result.LoadBalancerDescriptions {
		lbList = append(lbList, *(ln.LoadBalancerName))
		lbDnsList = append(lbDnsList, *(ln.DNSName))
	}
	log.Println("Total Number of load balancers: ", len(result.LoadBalancerDescriptions))
	log.Println("Name of load balancers: ", lbList)
	return svc, lbList, lbDnsList

}

func MatchLBTags(tagKeys []string, tagValues []string) (lb_name string) {
	svc, lbList, dnsList := GetLBNameList()
	var listLBName []*string
	var dnsName string
	j := 0
	for _, lbName := range lbList {
		isFound := true
		listLBName = append(listLBName, &lbName)
		dnsName = dnsList[j]
		input := &elb.DescribeTagsInput{
			LoadBalancerNames: listLBName,
		}
		lb_tags, _ := svc.DescribeTags(input)
		log.Println("Number of tags on load balancer: ", lbName, len(lb_tags.TagDescriptions[0].Tags))

		for i := 0; i < len(tagKeys); i++ {
			if !isFound {
				break
			}
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
			return dnsName
		}
		j++

	}
	return "Not found"
}
func OpenRepo(gitDir string, gitRepo string) (gr *git.Repository) {
	publicKey := util.GetAccessKey()
	re, e := git.PlainOpen(gitDir)
	if e != nil {
		log.Print("Not able to open any existing repo.")
	}
	if re == nil {
		re, e = git.PlainClone(gitDir, false, &git.CloneOptions{
			URL:        gitRepo,
			Progress:   os.Stdout,
			Auth:       publicKey,
			RemoteName: "master",
		})
		if e != nil {
			log.Fatal("Error. Failed to clone the repo", e.Error())
		} else {
			log.Println("Cloned the repo")
		}
		//fmt.Println(repo.Config())
	} else {
		log.Println("Repo is existing already, Refreshing it.")
		wt, err := re.Worktree()
		if err == nil {
			err = wt.Pull(&git.PullOptions{
				RemoteName: "master",
				Progress:   os.Stdout,
				Auth:       publicKey,
			})
			if err == nil {
				log.Println("Git repo pull is successful.")
			} else if err.Error() == "already up-to-date" {
				log.Println("Git repo is upto date: ", err.Error())
			} else {
				log.Fatal("Unable to refresh the repo from remote!", err.Error())
			}
		} else {
			log.Fatal("Unable to Create the worktree from from repo!")
		}
	}
	return re
}

func CreateBranch(gitDir string, repo *git.Repository) (br string) {
	branch := fmt.Sprintf("refs/heads/gitops-operator-branch-%s", strconv.FormatInt(time.Now().UnixNano(), 15))

	//_ = plumbing.ReferenceName("refs/heads/my-branch")
	b := plumbing.NewBranchReferenceName(branch)
	wt, err := repo.Worktree()

	err = wt.Checkout(&git.CheckoutOptions{Create: false, Force: false, Branch: b})

	if err != nil {
		// got an error  - try to create it
		log.Println("Unable to checkout the branch: ", branch, err.Error())
		err := wt.Checkout(&git.CheckoutOptions{Create: true, Force: false, Branch: b})
		if err != nil {
			log.Println("Unable to create the branch: ", branch, err.Error())
		} else {
			log.Println("New Branch created successfully: ", branch)
		}
	}
	return branch
}
func CreatePR(gitDir string, repo *git.Repository, refSpec string) (isPR bool) {
	pk := util.GetAccessKey()
	wt, err := repo.Worktree()
	_, err = wt.Add(".")
	if err != nil {
		log.Fatal("Unable to add the file into index.", err.Error())
	}
	_, err = wt.Commit("digibank gitops operator", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Gitops operator",
			Email: "gitops@digibank.org",
			When:  time.Now(),
		},
	})
	if err != nil {
		log.Fatal("Unable to commit on the git branch.", err.Error())
	} else {
		log.Println("Success! commit done on the branch.")
	}
	rs := config.RefSpec("refs/heads/*:refs/heads/*")
	err = repo.Push(&git.PushOptions{
		RemoteName: "master",
		RefSpecs:   []config.RefSpec{rs},
		Auth:       pk,
		Progress:   os.Stdout,
	})
	if err != nil {
		log.Fatal("Unable to Push the branch to remote.", err.Error())
	} else {
		log.Println("Success! Pushed the changes.")
	}
	err = wt.Checkout(&git.CheckoutOptions{Create: false, Force: false, Branch: "refs/heads/master"})
	if err != nil {
		log.Fatal("Unable to Check back the master branch out.", err.Error())
	} else {
		log.Println("Success! Checked out the master branch!")
		return true
	}
	return false

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

	tagKeys, tagValues := util.ParseTagKeyValues(gitAgent)

	lbName := MatchLBTags(tagKeys, tagValues)

	var gitMountDir = "/tmp/sgbank-terraformer"
	var gitRepo = gitAgent.Spec.GitInfo["url"]
	var gitFile = gitAgent.Spec.GitInfo["file"]
	var keyVar = gitAgent.Spec.GitInfo["keyvar"]
	var separator = gitAgent.Spec.GitInfo["fs"]
	codeConfigStatus := gitAgent.Status.CodeConfigStatus

	repo := OpenRepo(gitMountDir, gitRepo)
	changed := util.UpdateSourceCode(gitMountDir, gitFile, keyVar, separator, lbName)

	if changed && codeConfigStatus != "PR Created" {
		refSpec := CreateBranch(gitMountDir, repo)
		util.CopyFile(gitMountDir, gitFile, keyVar)
		_ = CreatePR(gitMountDir, repo, refSpec)
		gitAgent.Status.CodeConfigStatus = "PR Created"

	} else if changed && codeConfigStatus == "PR Created" {
		log.Info("Alert! PR has been created already. Please merge the changes.")
	} else if !changed {
		log.Info("Code and Config is in sync. No change required.")
		gitAgent.Status.CodeConfigStatus = "In Sync"
	}
	gitAgent.Status.AwsComponentName = lbName

	err = r.Status().Update(ctx, gitAgent)
	if err != nil {
		log.Error(err, "Failed to update GitAgent status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Second * 60, Requeue: true}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GitagentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&awsv1alpha1.Gitagent{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
