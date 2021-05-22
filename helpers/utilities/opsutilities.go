// Generic helper functions for operator
package utilities

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"

	awsv1alpha1 "github.com/gitops/gitops-operator/api/v1alpha1"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

func ParseTagKeyValues(gitAgent *awsv1alpha1.Gitagent) (tk []string, tv []string) {
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

func GetAccessKey() (pk *ssh.PublicKeys) {
	var publicKey *ssh.PublicKeys
	sshPath := os.Getenv("HOME") + "/.ssh/id_rsa"
	sshKey, _ := ioutil.ReadFile(sshPath)
	publicKey, keyError := ssh.NewPublicKeys("git", []byte(sshKey), "")
	if keyError != nil {
		fmt.Println(keyError)
	}
	return publicKey
}

func UpdateSourceCode(gitDir string, gitFile string, keyVar string, separator string, lbName string) (iv bool) {
	//var tmpOutputFile = "/tmp/gitops"
	of, err := os.Create("/tmp/gitops" + keyVar)
	defer of.Close()
	if err != nil {
		log.Println(err.Error(), "Error! Not able to open the tmp file")
	}

	var changed bool
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
			lineValues := strings.Split(line, separator)
			itemValue := lineValues[1]
			itemValue = strings.TrimSpace(itemValue)
			itemValue = strings.Trim(itemValue, "\"") //Trim the double quotes
			itemValue = strings.Trim(itemValue, "'")  //Trim the single quotes
			log.Println("Values in source code for given variable: ", lineValues[0], itemValue)
			if itemValue != lbName {
				changed = true
				log.Println("Mismatch in code vs config! Uptaing the Code.")
			}
			line = strings.Replace(line, itemValue, lbName, 1)
		}
		//fmt.Println(line)
		of.WriteString(line + "\n")
	}
	return changed
}
func CopyFile(gitDir string, gitFile string, keyVar string) {
	var tmpOutputFile = "/tmp/gitops" + keyVar
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
