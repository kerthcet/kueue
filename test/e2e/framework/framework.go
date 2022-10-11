package framework

import (
	"context"

	"github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	kueue "sigs.k8s.io/kueue/apis/kueue/v1alpha2"
	"sigs.k8s.io/kueue/pkg/util/testing"
	//+kubebuilder:scaffold:imports
)

func CreateClientUsingCluster() client.Client {

	cfg := config.GetConfigOrDie()
	gomega.ExpectWithOffset(1, cfg).NotTo(gomega.BeNil())

	err := kueue.AddToScheme(scheme.Scheme)
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())

	// +kubebuilder:scaffold:scheme
	client, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	gomega.ExpectWithOffset(1, err).NotTo(gomega.HaveOccurred())
	return client

}

func KueueReadyForTesting(client client.Client) {
	// To verify that webhooks is ready, let's create a simple resourceflavor
	resourceKueue := testing.MakeResourceFlavor("default").Obj()
	gomega.Eventually(func() bool {
		err := client.Create(context.Background(), resourceKueue)
		return err == nil
	}, Timeout, Interval).Should(gomega.BeTrue())
	gomega.Expect(client.Delete(context.Background(), resourceKueue)).Should(gomega.Succeed())

}
