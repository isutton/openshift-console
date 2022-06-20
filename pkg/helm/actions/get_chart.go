package actions

import (
	"fmt"
	"os"
	"strings"

	"github.com/openshift/console/pkg/helm/chartproxy"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"

	"k8s.io/client-go/dynamic"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

// constants
const (
	configNamespace         = "openshift-config"
	tlsSecretCertKey        = "tls.crt"
	tlsSecretKey            = "tls.key"
	caBundleKey             = "ca-bundle.crt"
	tlsSecretPattern        = "tlscrt-*"
	tlsKeyPattern           = "tlskey-*"
	cacertPattern           = "cacert-*"
	openshiftRepoUrl        = "https://charts.openshift.io"
	chartRepoPrefix         = "chart.openshift.io/chart-url-prefix"
	openshiftChartUrlPrefix = "https://github.com/openshift-helm-charts/"
)

func writeTempFile(data []byte, pattern string) (*os.File, error) {
	f, createErr := os.CreateTemp("", pattern)
	if createErr != nil {
		return nil, createErr
	}

	_, writeErr := f.Write(data)
	if writeErr != nil {
		return nil, writeErr
	}

	closeErr := f.Close()
	if closeErr != nil {
		return nil, closeErr
	}

	return f, nil
}

func getRepositoryNameAndNamespaceFromChartUrl(url, namespace string, client dynamic.Interface, coreClient corev1client.CoreV1Interface) (string, string, error) {
	repoGetter := chartproxy.NewRepoGetter(client, coreClient)
	helmRepo, err := repoGetter.List(namespace)
	if err != nil {
		return "", "", fmt.Errorf("Error In Finding the chart repositories")
	}
	//iterate the chart repo and find the repo with url
	for _, repo := range helmRepo {
		val, ok := repo.Annotations[chartRepoPrefix]
		if ok && strings.HasPrefix(url, val) {
			return repo.Name, repo.Namespace, nil
		} else if strings.HasPrefix(url, repo.URL.String()) {
			return repo.Name, repo.Namespace, nil
		}
	}
	return "", "", fmt.Errorf("Prefix Not Found")
}

func GetChart(url string, conf *action.Configuration, ns string, client dynamic.Interface, coreClient corev1client.CoreV1Interface, filesCleanup bool) (*chart.Chart, error) {
	cmd := action.NewInstall(conf)
	repositoryName, repositoryNamespace, err := getRepositoryNameAndNamespaceFromChartUrl(url, ns, client, coreClient)
	if err != nil {
		return nil, err
	}
	connectionConfig, err := getRepoConnectionConfig(repositoryName, repositoryNamespace, client)
	if err != nil {
		return nil, err
	}
	// tlsFiles contain references of files to be removed once the chart
	// operation depending on those files is finished.
	tlsFiles, err := setUpAuthentication(cmd, connectionConfig, coreClient)
	cmd.ChartPathOptions.RepoURL = connectionConfig.URL
	// downloads and caches the chart from the given url
	paths := strings.Split(url, "/")
	names := strings.Split(paths[len(paths)-1], "-")
	fmt.Println("------------------")
	fmt.Println(names[0], cmd.ChartPathOptions.RepoURL)
	fmt.Println("------------------")
	chartLocation, locateChartErr := cmd.ChartPathOptions.LocateChart(names[0], settings)
	fmt.Println("Locate Error", locateChartErr)
	if locateChartErr != nil {
		return nil, locateChartErr
	}
	defer func() {
		if filesCleanup == false {
			for _, f := range tlsFiles {
				fmt.Println(f.Name())
			}
			return
		}
		for _, f := range tlsFiles {
			os.Remove(f.Name())
		}
	}()
	return loader.Load(chartLocation)
}
