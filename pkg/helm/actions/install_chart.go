package actions

import (
	"context"
	"fmt"
	"os"

	"github.com/openshift/api/helm/v1beta1"
	"github.com/openshift/console/pkg/helm/metrics"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func InstallChart(ns, name, url string, vals map[string]interface{}, conf *action.Configuration, client dynamic.Interface, coreClient corev1client.CoreV1Interface, fileCleanUp bool, repositoryName string) (*release.Release, error) {
	fmt.Println("Name", name)
	fmt.Println("Url", url)
	var err error
	var chartInfo *ChartInfo
	cmd := action.NewInstall(conf)
	// tlsFiles contain references of files to be removed once the chart
	// operation depending on those files is finished.
	tlsFiles := []*os.File{}
	if repositoryName == "" {
		chartInfo, err = getChartInfoFromChartUrl(url, ns, client, coreClient)
		if err != nil {
			return nil, err
		}
	}
	cmd.ChartPathOptions.Version = chartInfo.Version

	connectionConfig, err := getRepositoryConnectionConfig(repositoryName, ns, client)
	if err != nil {
		return nil, err
	}
	cmd.ChartPathOptions.RepoURL = connectionConfig.URL

	tlsFiles, err = setUpAuthentication(cmd.ChartPathOptions, connectionConfig, coreClient)
	if err != nil {
		return nil, err
	}
	cmd.ReleaseName = name
	cp, err := cmd.ChartPathOptions.LocateChart(chartInfo.Name, settings)
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
		if fileCleanUp == false {
			return
		}
		for _, f := range tlsFiles {
			os.Remove(f.Name())
		}
	}()
	return release, nil
}

// getRepositoryConnectionConfig returns the connection configuration for the
// repository with given `name` and `namespace`.
func getRepositoryConnectionConfig(
	name string,
	namespace string,
	client dynamic.Interface,
) (*v1beta1.ConnectionConfig, error) {
	// attempt to get a project scoped Helm Chart repository
	unstructuredRepository, getProjectRepositoryErr := client.Resource(helmChartRepositoryNamespaceGVK).Namespace(namespace).Get(context.TODO(), name, v1.GetOptions{})
	if getProjectRepositoryErr == nil {
		var repository v1beta1.ProjectHelmChartRepository
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredRepository.Object, &repository)
		if err != nil {
			return nil, err
		}
		return &repository.Spec.ConnectionConfig, nil
	}

	// attempt to get a cluster scoped Helm Chart repository
	unstructuredRepository, getClusterRepositoryErr := client.Resource(helmChartRepositoryClusterGVK).Get(context.TODO(), name, v1.GetOptions{})
	if getClusterRepositoryErr == nil {
		var repository v1beta1.HelmChartRepository
		err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredRepository.Object, &repository)
		if err != nil {
			return nil, err
		}
		return &repository.Spec.ConnectionConfig, nil
	}

	// neither project or cluster scoped Helm Chart repositories have been found.
	klog.Errorf("Error listing namespace helm chart repositories: %v \nempty repository list will be used", getClusterRepositoryErr)
	return nil, getClusterRepositoryErr
}
