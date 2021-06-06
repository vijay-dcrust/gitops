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
	"context"
	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	awsv1alpha1 "github.com/gitops/gitops-operator/api/v1alpha1"
	aws "github.com/gitops/gitops-operator/helpers/aws"
	git "github.com/gitops/gitops-operator/helpers/git"
	util "github.com/gitops/gitops-operator/helpers/utilities"

	"time"
)

const (
	// ProviderName is the cloud provider providing loadbalancing functionality
	ProviderName = "aws"
)

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

	lbName := aws.MatchLBTags(tagKeys, tagValues)

	var gitMountDir = "/tmp/sgbank-terraformer"
	var gitRepo = gitAgent.Spec.GitInfo["url"]
	var gitFile = gitAgent.Spec.GitInfo["file"]
	var keyVar = gitAgent.Spec.GitInfo["keyvar"]
	var separator = gitAgent.Spec.GitInfo["fs"]
	codeConfigStatus := gitAgent.Status.CodeConfigStatus

	repo := git.OpenRepo(gitMountDir, gitRepo)
	changed := util.UpdateSourceCode(gitMountDir, gitFile, keyVar, separator, lbName)

	if changed && codeConfigStatus != "PR Created" {
		refSpec := git.CreateBranch(gitMountDir, repo)
		util.CopyFile(gitMountDir, gitFile, keyVar)
		_ = git.CreatePR(gitMountDir, repo, refSpec)
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
