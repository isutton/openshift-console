package actions

import (
	"fmt"
	"os"
	"strings"
	"unicode"

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

// writeTempFile creates a temporary file with the given `data`. `pattern`
// is used by `os.CreateTemp` to create a file in the filesystem.
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

// getRepositoryNameAndNamespaceFromChartUrl returns the name and namespace of
// the repository containing the given `url`.
//
// This function works by listing all available Helm Chart repositories (either
// scoped by the given `namespace` or cluster scoped), then comparing URLs of
// all existing charts in the repository manifest to match the given `chartUrl`.
func getRepositoryNameAndNamespaceFromChartUrl(
	chartUrl string,
	namespace string,
	client dynamic.Interface,
	coreClient corev1client.CoreV1Interface,
) (string, string, error) {
	repositories, err := chartproxy.NewRepoGetter(client, coreClient).List(namespace)
	if err != nil {
		return "", "", fmt.Errorf("error listing repositories: %w", err)
	}

	for _, repository := range repositories {
		idx, err := repository.IndexFile()
		if err != nil {
			return "", "", fmt.Errorf("error producing the index file of repository %q in namespace %q", repository.Name, repository.Namespace)
		}
		for _, chartVersions := range idx.Entries {
			for _, chartVersion := range chartVersions {
				for _, url := range chartVersion.URLs {
					if chartUrl == url {
						return repository.Name, repository.Namespace, nil
					}
				}
			}
		}
	}
	return "", "", fmt.Errorf("could not find a repository for the chart url %q in namespace %q", chartUrl, namespace)
}
func FindStartOfIndex(chartNameWithTarRef string) int {
	for i := 1; i < len(chartNameWithTarRef); i++ {
		if chartNameWithTarRef[i-1] == '-' && unicode.IsNumber(rune(chartNameWithTarRef[i])) {
			return i - 1
		}
	}
	return 0
}
func getChartNameFromUrl(url string) string {
	paths := strings.Split(url, "/")
	startOfTar := FindStartOfIndex(paths[len(paths)-1])
	//names := strings.Split(paths[len(paths)-1], "-")
	fmt.Println("------------------")
	fmt.Println(paths[len(paths)-1][0:startOfTar])
	fmt.Println("------------------")
	return paths[len(paths)-1][0:startOfTar]
}
func GetChart(url string, conf *action.Configuration, repositoryNamespace string, client dynamic.Interface, coreClient corev1client.CoreV1Interface, filesCleanup bool, repositoryName string) (*chart.Chart, error) {
	var err error
	cmd := action.NewInstall(conf)
	if repositoryName == "" || repositoryNamespace == "" {
		repositoryName, repositoryNamespace, err = getRepositoryNameAndNamespaceFromChartUrl(url, repositoryNamespace, client, coreClient)
		if err != nil {
			return nil, err
		}
	}

	connectionConfig, err := getRepositoryConnectionConfig(repositoryName, repositoryNamespace, client)
	if err != nil {
		return nil, err
	}
	// tlsFiles contain references of files to be removed once the chart
	// operation depending on those files is finished.
	tlsFiles, err := setUpAuthentication(cmd, connectionConfig, coreClient)
	cmd.ChartPathOptions.RepoURL = connectionConfig.URL
	// downloads and caches the chart from the given url
	chartName := getChartNameFromUrl(url)
	chartLocation, locateChartErr := cmd.ChartPathOptions.LocateChart(chartName, settings)
	fmt.Println("Locate Error", locateChartErr)
	if locateChartErr != nil {
		return nil, locateChartErr
	}
	defer func() {
		if filesCleanup == false {
			return
		}
		for _, f := range tlsFiles {
			os.Remove(f.Name())
		}
	}()
	return loader.Load(chartLocation)
}
