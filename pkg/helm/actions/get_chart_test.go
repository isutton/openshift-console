package actions

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	fk "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func setSettings(settings *cli.EnvSettings) {
	settings.RepositoryCache = "temporary"
	settings.RegistryConfig = "temporaryRegistryConfig"
	settings.RepositoryConfig = "/temporaryRepositoryConfig"
}
func TestGetChartWithoutTls(t *testing.T) {
	setSettings(settings)
	err := ExecuteScript("./testdata/chartmuseumWithoutTls.sh")
	require.NoError(t, err)
	err = ExecuteScript("./testdata/uploadChartsWithoutTls.sh")
	fmt.Println(err)
	tests := []struct {
		name      string
		chartPath string
		chartName string
		errorMsg  string
		namespace string
		helmCRS   []*unstructured.Unstructured
	}{
		{
			name:      "Valid chart url",
			chartPath: "http://localhost:8080/charts/mariadb-7.3.5.tgz",
			chartName: "mariadb",
			namespace: "",
			helmCRS: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "helm.openshift.io/v1beta1",
						"kind":       "HelmChartRepository",
						"metadata": map[string]interface{}{
							"name": "without-tls",
						},
						"spec": map[string]interface{}{
							"connectionConfig": map[string]interface{}{
								"url": "http://localhost:8080",
							},
						},
					},
				},
			},
		},
		{
			name:      "Invalid chart url",
			chartPath: "../testdata/invalid.tgz",
			errorMsg:  `Chart Not Found`,
		},
		{
			name:      "Not Valid chart url",
			chartPath: "http://localhost:8080/charts/mariadb-7.3.6.tgz",
			chartName: "mariadb",
			namespace: "",
			errorMsg:  `Chart Not Found`,
			helmCRS: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "helm.openshift.io/v1beta1",
						"kind":       "HelmChartRepository",
						"metadata": map[string]interface{}{
							"name": "without-tls",
						},
						"spec": map[string]interface{}{
							"connectionConfig": map[string]interface{}{
								"url": "http://localhost:8080",
							},
						},
					},
				},
			},
		},
	}
	store := storage.Init(driver.NewMemory())
	actionConfig := &action.Configuration{
		RESTClientGetter: FakeConfig{},
		Releases:         store,
		KubeClient:       &kubefake.PrintingKubeClient{Out: ioutil.Discard},
		Capabilities:     chartutil.DefaultCapabilities,
		Log:              func(format string, v ...interface{}) {},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := K8sDynamicClientFromCRs(test.helmCRS...)
			clientInterface := k8sfake.NewSimpleClientset()
			coreClient := clientInterface.CoreV1()
			chart, err := GetChart(test.chartPath, actionConfig, test.namespace, client, coreClient, true, "")
			fmt.Println(err)
			if test.name == "Not Valid chart url" {
				require.Error(t, err)
			}
			if err != nil && err.Error() != test.errorMsg {
				t.Errorf("Expected error %s but got %s", test.errorMsg, err.Error())
			}
			if err == nil && chart.Metadata.Name != test.chartName {
				t.Errorf("Expected chart name %s but got %s", test.chartName, chart.Metadata.Name)
			}
		})
	}
	err = ExecuteScript("./testdata/cleanupNonTls.sh")
	require.NoError(t, err)
}
func ExecuteScript(filepath string) error {
	tlsCmd := exec.Command(filepath)
	tlsCmd.Stdout = os.Stdout
	err := tlsCmd.Start()
	if err != nil {
		return err
	}
	if filepath != "./testdata/chartmuseum.sh" && filepath != "./testdata/chartmuseumWithoutTls.sh" {
		err = tlsCmd.Wait()
		if err != nil {
			return err
		}
	}
	return nil
}
func TestGetChartWithTlsData(t *testing.T) {
	setSettings(settings)
	// os.Setenv("HELM_REPOSITORY_CACHE", helmpath.CachePath("tmp/repository"))
	//create the server.key and server.crt
	err := ExecuteScript("./testdata/createTlsSecrets.sh")
	require.NoError(t, err)
	//start chartmuseum server
	err = ExecuteScript("./testdata/chartmuseum.sh")
	require.NoError(t, err)
	err = ExecuteScript("./testdata/cacertCreate.sh")
	fmt.Println(err)
	require.NoError(t, err)
	err = ExecuteScript("./testdata/uploadCharts.sh")
	tests := []struct {
		name                string
		chartPath           string
		chartName           string
		repositoryName      string
		repositoryNamespace string
		createSecret        bool
		createNamespace     bool
		createHelmRepo      bool
		namespace           string
		createConfigMap     bool
		errorMsg            string
		helmCRS             []*unstructured.Unstructured
	}{
		{
			name:            "mychart",
			chartPath:       "https://localhost:8443/charts/mychart-0.1.0.tgz",
			chartName:       "mychart",
			createSecret:    true,
			createNamespace: true,
			createConfigMap: true,
			namespace:       "test",
			createHelmRepo:  true,
			helmCRS: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "helm.openshift.io/v1beta1",
						"kind":       "ProjectHelmChartRepository",
						"metadata": map[string]interface{}{
							"namespace": "test",
							"name":      "my-repo",
						},
						"spec": map[string]interface{}{
							"connectionConfig": map[string]interface{}{
								"url": "https://localhost:8443",
								"tlsClientConfig": map[string]interface{}{
									"name":      "my-repo",
									"namespace": "test",
								},
								"ca": map[string]interface{}{
									"name": "my-repo",
								},
							},
						},
					},
				},
			},
		},
		{
			name:            "mariadb",
			chartPath:       "https://localhost:8443/charts/mariadb-7.3.5.tgz",
			chartName:       "mariadb",
			repositoryName:  "my-repo",
			createHelmRepo:  true,
			createSecret:    true,
			createNamespace: true,
			createConfigMap: true,
			helmCRS: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "helm.openshift.io/v1beta1",
						"kind":       "HelmChartRepository",
						"metadata": map[string]interface{}{
							"name": "my-repo",
						},
						"spec": map[string]interface{}{
							"connectionConfig": map[string]interface{}{
								"url": "https://localhost:8443",
								"tlsClientConfig": map[string]interface{}{
									"name": "my-repo",
								},
								"ca": map[string]interface{}{
									"name": "my-repo",
								},
							},
						},
					},
				},
			},
		},
		{
			name:           "Invalid chart url",
			chartPath:      "../testdata/invalid.tgz",
			errorMsg:       `Chart Not Found`,
			repositoryName: "",
		},
	}
	store := storage.Init(driver.NewMemory())
	actionConfig := &action.Configuration{
		RESTClientGetter: FakeConfig{},
		Releases:         store,
		KubeClient:       &kubefake.PrintingKubeClient{Out: ioutil.Discard},
		Capabilities:     chartutil.DefaultCapabilities,
		Log:              func(format string, v ...interface{}) {},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			objs := []runtime.Object{}
			// create a namespace if it is not same as openshift-config
			if test.createNamespace && test.namespace != configNamespace {
				nsSpec := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: test.namespace}}
				objs = append(objs, nsSpec)
			}
			// create a secret in required namespace
			if test.createSecret {
				certificate, errCert := ioutil.ReadFile("./server.crt")
				require.NoError(t, errCert)
				key, errKey := ioutil.ReadFile("./server.key")
				require.NoError(t, errKey)
				data := map[string][]byte{
					"tls.key": key,
					"tls.crt": certificate,
				}
				if test.namespace == "" {
					test.namespace = configNamespace
				}
				secretSpec := &v1.Secret{Data: data, ObjectMeta: metav1.ObjectMeta{Name: "my-repo", Namespace: test.namespace}}
				objs = append(objs, secretSpec)
			}
			//create a configMap in openshift-config namespace
			if test.createConfigMap {
				caCert, err := ioutil.ReadFile("./cacert.pem")
				require.NoError(t, err)
				data := map[string]string{
					"ca-bundle.crt": string(caCert),
				}
				secretSpec := &v1.ConfigMap{Data: data, ObjectMeta: metav1.ObjectMeta{Name: "my-repo", Namespace: configNamespace}}
				objs = append(objs, secretSpec)
			}

			client := K8sDynamicClientFromCRs(test.helmCRS...)
			clientInterface := k8sfake.NewSimpleClientset(objs...)
			coreClient := clientInterface.CoreV1()
			chart, err := GetChart(test.chartPath, actionConfig, test.namespace, client, coreClient, false, "")
			if test.errorMsg != "" {
				require.Equal(t, test.errorMsg, err.Error())
			} else {
				require.NoError(t, err)
				require.NotNil(t, chart.Metadata)
				require.Equal(t, chart.Metadata.Name, test.chartName)
			}
		})
	}
	err = ExecuteScript("./testdata/cleanup.sh")
	require.NoError(t, err)
}

func K8sDynamicClientFromCRs(crs ...*unstructured.Unstructured) dynamic.Interface {
	var objs []runtime.Object

	for _, cr := range crs {
		objs = append(objs, cr)
	}
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "helm.openshift.io", Version: "v1beta1", Kind: "HelmChartRepositoryList"}, &unstructured.UnstructuredList{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "helm.openshift.io", Version: "v1beta1", Kind: "ProjectHelmChartRepositoryList"}, &unstructured.UnstructuredList{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "helm.openshift.io", Version: "v1beta1", Kind: "HelmChartRepository"}, &unstructured.Unstructured{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "helm.openshift.io", Version: "v1beta1", Kind: "ProjectHelmChartRepository"}, &unstructured.Unstructured{})
	return fk.NewSimpleDynamicClient(scheme, objs...)
}
