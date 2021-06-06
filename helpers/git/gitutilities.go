package git

import (
	"fmt"
	util "github.com/gitops/gitops-operator/helpers/utilities"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"log"
	"os"
	"strconv"
	"time"
)

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
