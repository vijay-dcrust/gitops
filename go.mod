module github.com/gitops/gitops-operator

go 1.15

require (
	github.com/aws/aws-sdk-go v1.38.36
	github.com/go-logr/logr v0.3.0
	github.com/google/martian v2.1.0+incompatible
	github.com/onsi/ginkgo v1.14.1
	github.com/onsi/gomega v1.10.2
	k8s.io/apimachinery v0.20.2
	k8s.io/client-go v0.20.2
	sigs.k8s.io/controller-runtime v0.8.3
)