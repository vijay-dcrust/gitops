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
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"

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
	isFound := false
	j := 0
	for _, lbName := range lbList {
		listLBName = append(listLBName, &lbName)
		dnsName = dnsList[j]
		input := &elb.DescribeTagsInput{
			LoadBalancerNames: listLBName,
		}
		lb_tags, _ := svc.DescribeTags(input)
		log.Println("Number of tags on load balancer: ", lbName, len(lb_tags.TagDescriptions[0].Tags))

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
			return dnsName
		}
		j++
	}
	return "Not found"
}

func OpenRepo(gitDir string, gitRepo string) (gr *git.Repository, pk *ssh.PublicKeys) {
	var publicKey *ssh.PublicKeys
	sshPath := os.Getenv("HOME") + "/.ssh/id_rsa"
	sshKey, _ := ioutil.ReadFile(sshPath)
	publicKey, keyError := ssh.NewPublicKeys("git", []byte(sshKey), "")
	if keyError != nil {
		fmt.Println(keyError)
	}
	re, e := git.PlainOpen(gitDir)
	if e != nil {
		log.Print("Not able to open any existing repo.")
	}
	if re == nil {
		re, e = git.PlainClone(gitDir, false, &git.CloneOptions{
			URL:      gitRepo,
			Progress: os.Stdout,
			Auth:     publicKey,
		})
		if e != nil {
			log.Print("Error. Failed to clone the repo")
		}
		log.Print("Cloned the repo")
		//fmt.Println(repo.Config())
	} else {
		log.Print("Repo is existing already, Refreshing it.")
		wt, err := re.Worktree()
		if err == nil {
			err = wt.Pull(&git.PullOptions{
				RemoteName: "origin",
				Auth:       publicKey,
			})
			if err == nil {
				log.Println("Git repo pull is successful.")
			} else if err.Error() == "already up-to-date" {
				log.Print("Git repo is upto date: ", err.Error())
			} else {
				log.Fatal("Unable to refresh the repo from remote!", err.Error())
			}
		} else {
			log.Fatal("Unable to Create the worktree from from repo!")
		}
	}
	return re, publicKey
}

func UpdateSourceCode(gitDir string, gitFile string, keyVar string, separator string, lbName string) (iv bool) {
	//var tmpOutputFile = "/tmp/gitops"
	of, err := os.Create("/tmp/gitops")
	defer of.Close()
	if err != nil {
		log.Println(err.Error(), "Error! Not able to open the tmp file")
	}

	var found = false
	file, err := os.Open(gitDir + "/" + gitFile)
	if err != nil {
		log.Println(err.Error(), "Error! Not able to open the file")
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		matched, _ := regexp.MatchString("\\b"+keyVar+"\\b", line)
		if matched {
			found = true
			lineValues := strings.Split(line, separator)
			itemValue := lineValues[1]
			itemValue = strings.TrimSpace(itemValue)
			itemValue = strings.Trim(itemValue, "\"") //Trim the double quotes
			itemValue = strings.Trim(itemValue, "'")  //Trim the single quotes
			fmt.Println("Values in source code for given variable: ", lineValues[0], itemValue)
			if itemValue != lbName {
				log.Println("Mismatch in code vs config! Uptaing the Code.")
			}
			line = strings.Replace(line, itemValue, lbName, 1)
		}
		//fmt.Println(line)
		of.WriteString(line + "\n")
	}
	return found
}
func CopyFile(gitDir string, gitFile string) {
	var tmpOutputFile = "/tmp/gitops"
	of, err := os.Open(tmpOutputFile)
	defer of.Close()
	if err != nil {
		log.Println(err.Error(), "Error! Not able to open the tmpOutputFile")
	}
	file, err := os.Create(gitDir + "/" + gitFile)
	if err != nil {
		log.Println(err.Error(), "Error! Not able to open the file")
	}
	defer file.Close()
	_, err = io.Copy(file, of)
	if err != nil {
		log.Fatal("Unable to copy the tmp file to :", gitFile, err.Error())
	}
	err = os.Remove(tmpOutputFile)
	if err != nil {
		log.Fatal("Unable to Delete the tmp file :", tmpOutputFile)
	}
}

/*
func CreatePR(gitDir string, repo *git.Repository, pk *ssh.PublicKeys) {
	headRef, err := repo.Head()
	ref := plumbing.NewHashReference("refs/heads/my-branch", headRef.Hash())

	err = repo.Storer.SetReference(ref)
	wt, err := repo.Worktree()
	err = wt.Checkout(&git.CheckoutOptions{
		Hash: plumbing.NewHash(commit),
	})

	_, err = wt.Add(gitDir + "/")

	commit, err := wt.Commit("example go-git commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Vijay Pal",
			Email: "vijay@db.org",
			When:  time.Now(),
		},
	})
	obj, err := repo.CommitObject(commit)
	fmt.Println(obj)
	err = repo.Push(&git.PushOptions{
		Progress: os.Stdout,
		Auth:     pk,
	})
	err = wt.Pull(&git.PullOptions{
		RemoteName: "origin",
		Auth:       pk,
	})

	if err != nil {
		log.Print("Not able to create a PR", err.Error())
	}
}
*/

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

	var gitMountDir = "/tmp/sgbank-terraformer"
	var gitRepo = "git@gitlab.myteksi.net:dakota/sgbank-terraform.git"
	var gitFile = "providers/aws/sg/central/dns/private_zone/terraform.tfvars"
	var keyVar = "dev-istio-controller-elb"
	var separator = "="
	_, _ = OpenRepo(gitMountDir, gitRepo)
	changed := UpdateSourceCode(gitMountDir, gitFile, keyVar, separator, lbName)
	if changed {
		CopyFile(gitMountDir, gitFile)
		//CreatePR(gitMountDir, repo, pk)
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
