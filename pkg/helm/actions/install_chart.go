package actions

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/openshift/api/helm/v1beta1"
	"github.com/openshift/console/pkg/helm/metrics"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/klog"
)

var (
	helmChartRepositoryClusterGVK = schema.GroupVersionResource{
		Group:    "helm.openshift.io",
		Version:  "v1beta1",
		Resource: "helmchartrepositories",
	}
	helmChartRepositoryNamespaceGVK = schema.GroupVersionResource{
		Group:    "helm.openshift.io",
		Version:  "v1beta1",
		Resource: "projecthelmchartrepositories",
	}
)

func InstallChart(ns, name, url string, vals map[string]interface{}, conf *action.Configuration, client dynamic.Interface, coreClient corev1client.CoreV1Interface) (*release.Release, error) {
	cmd := action.NewInstall(conf)
	// tlsFiles contain references of files to be removed once the chart
	// operation depending on those files is finished.
	tlsFiles := []*os.File{}
	var tlsConfigNamespace, configMapName, secretName string
	repositoryName, _, err := getChartNameAndNamespaceFromChartUrl(url, ns, client, coreClient)
	if err != nil {
		//serverutils.SendResponse(w, http.StatusBadGateway, serverutils.ApiError{Err: fmt.Sprintf("Failed to parse request: %v", err)})
		return nil, err
	}
	// Create a Kubernetes core/v1 client.
	connectionConfig, err := getRepoConnectionConfig(repositoryName, ns, client)
	if err != nil {
		//serverutils.SendResponse(w, http.StatusBadGateway, serverutils.ApiError{Err: fmt.Sprintf("Failed to parse request: %v", err)})
		return nil, err
	}
	if connectionConfig != (&v1beta1.ConnectionConfig{}) {
		if connectionConfig.TLSClientConfig != nil {
			secretName = connectionConfig.TLSClientConfig.Name
			tlsConfigNamespace = connectionConfig.TLSClientConfig.Namespace
			if tlsConfigNamespace == "" {
				tlsConfigNamespace = configNamespace
			}
		}
		if connectionConfig.CA != nil {
			configMapName = connectionConfig.CA.Name
		}

		if configMapName != "" {
			configMap, err := coreClient.ConfigMaps(configNamespace).Get(context.TODO(), configMapName, v1.GetOptions{})
			if err != nil {
				return nil, errors.New(fmt.Sprintf("Failed to GET configmap %s, reason %v", configMapName, err))
			}
			caBundleKey := "ca-bundle.crt"
			caCertBytes, found := configMap.Data[caBundleKey]
			if !found {
				return nil, errors.New(fmt.Sprintf("Failed to find %s key in configmap %s", caBundleKey, configMapName))
			}
			caCertFile, caCertGetErr := writeTempFile([]byte(caCertBytes), "cacert-*")
			if caCertGetErr != nil {
				return nil, caCertGetErr
			}
			cmd.ChartPathOptions.CaFile = caCertFile.Name()
			tlsFiles = append(tlsFiles, caCertFile)
		}
		if secretName != "" {
			secret, err := coreClient.Secrets(tlsConfigNamespace).Get(context.TODO(), secretName, v1.GetOptions{})
			if err != nil {
				return nil, errors.New(fmt.Sprintf("Failed to GET secret %s from %vreason %v", secretName, tlsConfigNamespace, err))
			}
			tlsCertSecretKey := "tls.crt"
			tlsCertBytes, found := secret.Data[tlsCertSecretKey]
			if !found {
				return nil, errors.New(fmt.Sprintf("Failed to find %s key in secret %s", tlsCertSecretKey, secretName))
			}
			tlsCertFile, err := writeTempFile((tlsCertBytes), "tlscrt-*")
			if err != nil {
				return nil, err
			}
			cmd.ChartPathOptions.CertFile = tlsCertFile.Name()
			tlsFiles = append(tlsFiles, tlsCertFile)
			tlsSecretKey := "tls.key"
			tlsKeyBytes, found := secret.Data[tlsSecretKey]
			if !found {
				return nil, errors.New(fmt.Sprintf("Failed to find %s key in secret %s", tlsSecretKey, secretName))
			}
			tlsKeyFile, err := writeTempFile(tlsKeyBytes, "tlskey-*")
			if err != nil {
				return nil, err
			}
			cmd.ChartPathOptions.KeyFile = tlsKeyFile.Name()
			tlsFiles = append(tlsFiles, tlsKeyFile)
		}
	}
	releaseName, chartName, err := cmd.NameAndChart([]string{name, url})
	if err != nil {
		return nil, err
	}
	cmd.ReleaseName = releaseName
	// add repo url
	//cmd.ChartPathOptions.RepoURL = connectionConfig.URL
	cp, err := cmd.ChartPathOptions.LocateChart(chartName, settings)
	if err != nil {
		return nil, err
	}

	ch, err := loader.Load(cp)
	if err != nil {
		return nil, err
	}

	// Add chart URL as an annotation before installation
	if ch.Metadata == nil {
		ch.Metadata = new(chart.Metadata)
	}
	if ch.Metadata.Annotations == nil {
		ch.Metadata.Annotations = make(map[string]string)
	}
	ch.Metadata.Annotations["chart_url"] = url

	cmd.Namespace = ns
	release, err := cmd.Run(ch, vals)
	if err != nil {
		return nil, err
	}

	if ch.Metadata.Name != "" && ch.Metadata.Version != "" {
		metrics.HandleconsoleHelmInstallsTotal(ch.Metadata.Name, ch.Metadata.Version)
	}
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
	return release, nil
}

func getRepoConnectionConfig(repoName, repoNamespace string, client dynamic.Interface) (*v1beta1.ConnectionConfig, error) {
	var err error
	var helmRepoUnstructured *unstructured.Unstructured
	helmRepoUnstructured, err = client.Resource(helmChartRepositoryNamespaceGVK).Namespace(repoNamespace).Get(context.TODO(), repoName, v1.GetOptions{})
	if err != nil {
		helmRepoUnstructured, err = client.Resource(helmChartRepositoryClusterGVK).Get(context.TODO(), repoName, v1.GetOptions{})
		if err != nil {
			klog.Errorf("Error listing namespace helm chart repositories: %v \nempty repository list will be used", err)
			return nil, err
		}
	}
	var helmRepo v1beta1.ProjectHelmChartRepository
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(helmRepoUnstructured.Object, &helmRepo)
	return &helmRepo.Spec.ConnectionConfig, nil
}
