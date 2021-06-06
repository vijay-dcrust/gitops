package aws

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	"log"
)

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
