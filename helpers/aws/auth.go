package aws

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
)

func GetAuth(region string) *aws.Config {
	awsConfig := &aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewSharedCredentials("", "sgdev"),
	}
	awsConfig = awsConfig.WithCredentialsChainVerboseErrors(true)
	return awsConfig
}
