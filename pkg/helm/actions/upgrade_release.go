package actions

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/openshift/api/helm/v1beta1"
	"github.com/openshift/console/pkg/helm/metrics"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

func setUpAuthenticationUpgrade(cmd *action.Upgrade, connectionConfig *v1beta1.ConnectionConfig, coreClient corev1client.CoreV1Interface) ([]*os.File, error) {
	tlsFiles := []*os.File{}
	var tlsConfigNamespace, configMapName, secretName string
	//set up tls cert and key
	if connectionConfig.TLSClientConfig != nil {
		secretName = connectionConfig.TLSClientConfig.Name
		tlsConfigNamespace = connectionConfig.TLSClientConfig.Namespace
		if tlsConfigNamespace == "" {
			tlsConfigNamespace = configNamespace
		}
		secret, err := coreClient.Secrets(tlsConfigNamespace).Get(context.TODO(), secretName, v1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("Failed to GET secret %s from %vreason %v", secretName, tlsConfigNamespace, err)
		}
		tlsCertBytes, found := secret.Data[tlsSecretCertKey]
		if !found {
			return nil, fmt.Errorf("Failed to find %s key in secret %s", tlsSecretCertKey, secretName)
		}
		tlsCertFile, err := writeTempFile((tlsCertBytes), tlsSecretPattern)
		if err != nil {
			return nil, err
		}
		cmd.ChartPathOptions.CertFile = tlsCertFile.Name()
		tlsFiles = append(tlsFiles, tlsCertFile)
		tlsKeyBytes, found := secret.Data[tlsSecretKey]
		if !found {
			return nil, fmt.Errorf("Failed to find %s key in secret %s", tlsSecretKey, secretName)
		}
		tlsKeyFile, err := writeTempFile(tlsKeyBytes, tlsKeyPattern)
		if err != nil {
			return nil, err
		}
		cmd.ChartPathOptions.KeyFile = tlsKeyFile.Name()
		tlsFiles = append(tlsFiles, tlsKeyFile)
	}
	//set up ca certificate
	if connectionConfig.CA != nil {
		configMapName = connectionConfig.CA.Name
		configMap, err := coreClient.ConfigMaps(configNamespace).Get(context.TODO(), configMapName, v1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("Failed to GET configmap %s, reason %v", configMapName, err)
		}
		caCertBytes, found := configMap.Data[caBundleKey]
		if !found {
			return nil, fmt.Errorf("Failed to find %s key in configmap %s", caBundleKey, configMapName)
		}
		caCertFile, caCertGetErr := writeTempFile([]byte(caCertBytes), "cacert-*")
		if caCertGetErr != nil {
			return nil, caCertGetErr
		}
		cmd.ChartPathOptions.CaFile = caCertFile.Name()
		tlsFiles = append(tlsFiles, caCertFile)
	}
	return tlsFiles, nil
}
func UpgradeRelease(ns, name, url string, vals map[string]interface{}, conf *action.Configuration, dynamicClient dynamic.Interface, coreClient corev1client.CoreV1Interface, fileCleanUp bool, repositoryName string) (*release.Release, error) {
	client := action.NewUpgrade(conf)
	client.Namespace = ns
	tlsFiles := []*os.File{}
	fmt.Println("-----------------------------")
	fmt.Println("Name", name)
	fmt.Println("UrL", url)
	fmt.Println("-----------------------------")
	var ch *chart.Chart
	var chartInfo *ChartInfo

	rel, err := GetRelease(name, conf)
	if err != nil {
		// if there is no release exist then return generic error
		if strings.Contains(err.Error(), "no revision for release") {
			return nil, ErrReleaseRevisionNotFound
		}
		return nil, err
	}

	// Before proceeding, check if chart URL is present as an annotation
	if rel.Chart.Metadata.Annotations != nil {
		if chart_url, ok := rel.Chart.Metadata.Annotations["chart_url"]; url == "" && ok {
			url = chart_url
		}
	}

	// if url is not provided then we expect user trying to upgrade release with the same
	// version of chart but with the different values
	if url == "" {
		ch = rel.Chart
	} else {
		if repositoryName == "" || ns == "" {
			chartInfo, err = getChartInfoFromChartUrl(url, ns, dynamicClient, coreClient)
			if err != nil {
				return nil, err
			}
		}

		connectionConfig, err := getRepositoryConnectionConfig(repositoryName, ns, dynamicClient)
		if err != nil {
			return nil, err
		}
		tlsFiles, err = setUpAuthenticationUpgrade(client, connectionConfig, coreClient)
		if err != nil {
			return nil, err
		}
		client.ChartPathOptions.RepoURL = connectionConfig.URL
		cp, err := client.ChartPathOptions.LocateChart(chartInfo.Name, settings)
		if err != nil {
			return nil, err
		}

		ch, err = loader.Load(cp)
		if err != nil {
			return nil, err
		}
	}

	if req := ch.Metadata.Dependencies; req != nil {
		if err := action.CheckDependencies(ch, req); err != nil {
			return nil, err
		}
	}

	// Ensure chart URL is properly set in the upgrade chart
	if url != "" {
		if ch.Metadata.Annotations == nil {
			ch.Metadata.Annotations = make(map[string]string)
		}
		ch.Metadata.Annotations["chart_url"] = url
	}

	rel, err = client.Run(name, ch, vals)
	if err != nil {
		return nil, err
	}

	if ch.Metadata.Name != "" && ch.Metadata.Version != "" {
		metrics.HandleconsoleHelmUpgradesTotal(ch.Metadata.Name, ch.Metadata.Version)
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
	return rel, nil
}
