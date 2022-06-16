package actions

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/openshift/console/pkg/helm/chartproxy"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/dynamic"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	configNamespace = "openshift-config"
)

// constants
const (
	tlsSecretCertKey = "tls.crt"
	tlsSecretKeyKey  = "tls.key"
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

func getChartNameAndNamespaceFromChartUrl(url, namespace string, client dynamic.Interface, coreClient corev1client.CoreV1Interface) (string, string, error) {
	// create an index.yaml from all repository
	//iterate all entries and find chart with the right url
	var repositoryName, repositoryNamespace string
	// indexFile, err := proxy.IndexFileAll(true)
	// if err != nil {
	// 	return "", "", fmt.Errorf("Error In Finding the chart repositories")
	// }
	// for _, entry := range indexFile.Entries {
	// 	for i := len(entry) - 1; i >= 0; i-- {
	// 		if containsUrl(url, entry[i].URLs) {
	// 			return entry[i].Annotations["repositoryName"], entry[i].Annotations["repositoryNamespace"], nil
	// 		}
	// 	}
	// }
	repoGetter := chartproxy.NewRepoGetter(client, coreClient)
	helmRepo, err := repoGetter.List(namespace)
	//helmRepo, err := FindAllRepositories(client, coreClient, namespace)
	if err != nil {
		return "", "", fmt.Errorf("Error In Finding the chart repositories")
	}
	for _, repo := range helmRepo {
		if strings.HasPrefix(url, repo.URL.String()) {
			return repo.Name, repo.Namespace, nil
		}
		//repositoryName, repositoryNamespace = FindChartWithUrl(url, idx)
		// if repositoryName != "" || repositoryNamespace != "" {
		// 	return repositoryName, repositoryNamespace, nil
		// }
	}
	return repositoryName, repositoryNamespace, fmt.Errorf("Not Found")
}

// func FindAllRepositories(client dynamic.Interface, coreClient corev1client.CoreV1Interface, namespace string) ([]*unstructured.Unstructured, error) {
// 	var helmRepo []*unstructured.Unstructured
// 	clusterRepos, err := client.Resource(helmChartRepositoryClusterGVK).Namespace(namespace).List(context.TODO(), v1.ListOptions{})
// 	if err != nil {
// 		return nil, err
// 	}
// 	for _, item := range clusterRepos.Items {
// 		helmConfig, err := b.unmarshallConfig(item)
// 		if err != nil {
// 			return nil, err
// 		}
// 		helmRepo = append(helmRepo, helmConfig)
// 	}
// 	namespaceRepos, err := client.Resource(helmChartRepositoryNamespaceGVK).List(context.TODO(), v1.ListOptions{})
// 	if err != nil {
// 		return nil, err
// 	}
// 	for _, item := range namespaceRepos.Items {
// 		helmConfig, err := b.unmarshallConfig(item)
// 		if err != nil {
// 			return nil, err
// 		}
// 		helmRepo = append(helmRepo, helmConfig)
// 	}
// 	return helmRepo, nil
// }

// func getIndexFile(repo *unstructured.Unstructured) *repo.IndexFile {
// 	return nil
// }

// func FindChartWithUrl(url string, indexFile *repo.IndexFile) (string, string) {
// 	var repositoryName, repositoryNamespace string
// 	for _, entry := range indexFile.Entries {
// 		for i := len(entry) - 1; i >= 0; i-- {
// 			if containsUrl(url, entry[i].URLs) {
// 				return entry[i].Annotations["repositoryName"], entry[i].Annotations["repositoryNamespace"]
// 			}
// 		}
// 	}
// 	return repositoryName, repositoryNamespace
// }

// func containsUrl(url string, urls []string) bool {
// 	for _, val := range urls {
// 		if val == url {
// 			return true
// 		}
// 	}
// 	return false
// }

func GetChart(url string, conf *action.Configuration, ns string, client dynamic.Interface, coreClient corev1client.CoreV1Interface) (*chart.Chart, error) {
	cmd := action.NewInstall(conf)
	repositoryName, repositoryNamespace, err := getChartNameAndNamespaceFromChartUrl(url, ns, client, coreClient)
	if err != nil {
		//serverutils.SendResponse(w, http.StatusBadGateway, serverutils.ApiError{Err: fmt.Sprintf("Failed to parse request: %v", err)})
		return nil, err
	}
	connectionConfig, err := getRepoConnectionConfig(repositoryName, repositoryNamespace, client)
	if err != nil {
		//serverutils.SendResponse(w, http.StatusBadGateway, serverutils.ApiError{Err: fmt.Sprintf("Failed to parse request: %v", err)})
		return nil, err
	}

	// tlsFiles contain references of files to be removed once the chart
	// operation depending on those files is finished.
	tlsFiles := []*os.File{}

	tlsClientConfig := connectionConfig.TLSClientConfig
	if tlsClientConfig != nil {
		if tlsClientConfig.Namespace == "" {
			tlsClientConfig.Namespace = configNamespace
		}
		if tlsClientConfig.Name != "" {
			tlsSecret, err := coreClient.Secrets(tlsClientConfig.Namespace).Get(context.Background(), tlsClientConfig.Name, v1.GetOptions{})
			if err != nil {
				return nil, fmt.Errorf("failed to GET secret %s from %v reason: %w", tlsClientConfig.Name, tlsClientConfig.Namespace, err)
			}

			//---------------------------------------------------------------
			// tls.key
			//---------------------------------------------------------------

			tlsKeyBytes, found := tlsSecret.Data[tlsSecretKeyKey]
			if !found {
				return nil, fmt.Errorf("failed to find %s key in secret %s", tlsSecretKeyKey, tlsClientConfig.Name)
			}
			tlsKeyFile, err := writeTempFile(tlsKeyBytes, "tlskey-*")
			if err != nil {
				return nil, err
			}
			cmd.ChartPathOptions.KeyFile = tlsKeyFile.Name()
			tlsFiles = append(tlsFiles, tlsKeyFile)

			//---------------------------------------------------------------
			// tls.crt
			//---------------------------------------------------------------

			tlsCertBytes, found := tlsSecret.Data[tlsSecretCertKey]
			if !found {
				return nil, fmt.Errorf("failed to find %s key in secret %s", tlsSecretCertKey, tlsClientConfig.Name)
			}
			tlsCertFile, err := writeTempFile((tlsCertBytes), "tlscrt-*")
			if err != nil {
				return nil, err
			}
			cmd.ChartPathOptions.CertFile = tlsCertFile.Name()
			tlsFiles = append(tlsFiles, tlsCertFile)
		}
	}

	if connectionConfig.CA != nil {
		caCertConfigMap, caCertGetErr := coreClient.ConfigMaps(configNamespace).Get(context.Background(), connectionConfig.CA.Name, v1.GetOptions{})
		if caCertGetErr != nil {
			return nil, fmt.Errorf("failed to GET configmap %s: %w", connectionConfig.CA.Name, caCertGetErr)
		}
		caBundleKey := "ca-bundle.crt"
		caCertBytes, found := caCertConfigMap.Data[caBundleKey]
		if !found {
			return nil, fmt.Errorf("failed to find %s key in configmap %s", caBundleKey, connectionConfig.CA.Name)
		}
		caCertFile, caCertGetErr := writeTempFile([]byte(caCertBytes), "cacert-*")
		if caCertGetErr != nil {
			return nil, caCertGetErr
		}
		cmd.ChartPathOptions.CaFile = caCertFile.Name()
		tlsFiles = append(tlsFiles, caCertFile)
	}

	//---------------------------------------------------------------
	// ca-bundle.crt
	//---------------------------------------------------------------
	// add repo url
	//cmd.ChartPathOptions.RepoURL = connectionConfig.URL
	// remove all the tls related files created by this process
	defer func() {
		if os.Getenv("HELM_CLEANUP") == "0" {
			for _, f := range tlsFiles {
				fmt.Println(f.Name())
			}
			return
		}
		for _, f := range tlsFiles {
			os.Remove(f.Name())
		}
	}()

	// downloads and caches the chart from the given url
	chartLocation, locateChartErr := cmd.ChartPathOptions.LocateChart(url, settings)
	if locateChartErr != nil {
		return nil, locateChartErr
	}

	return loader.Load(chartLocation)
}
