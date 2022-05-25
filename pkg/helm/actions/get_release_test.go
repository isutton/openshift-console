package actions

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/openshift/console/pkg/auth"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chartutil"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestGetRelease(t *testing.T) {
	err := ExecuteScript("./testdata/chartmuseumWithoutTls.sh")
	require.NoError(t, err)
	err = ExecuteScript("./testdata/uploadChartsWithoutTls.sh")
	require.NoError(t, err)
	tests := []struct {
		testName       string
		chartPath      string
		releaseName    string
		manifestValue  string
		repositoryName string
		helmCRS        []*unstructured.Unstructured
	}{
		{
			testName:       "valid chart path",
			chartPath:      "http://localhost:8080/charts/influxdb-3.0.2.tgz",
			releaseName:    "influxdb",
			manifestValue:  influxdbTemplateValue,
			repositoryName: "without-tls",
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
			testName:      "invalid chart path",
			chartPath:     "http://localhost:8080/influxdb-3.0.1.tgz",
			releaseName:   "influxdb-2",
			manifestValue: "",
		},
		{
			testName:      "invalid release name",
			chartPath:     "non-exist-path",
			releaseName:   "influxdb-non-exist",
			manifestValue: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			store := storage.Init(driver.NewMemory())
			actionConfig := &action.Configuration{
				RESTClientGetter: FakeConfig{},
				Releases:         store,
				KubeClient:       &kubefake.PrintingKubeClient{Out: ioutil.Discard},
				Capabilities:     chartutil.DefaultCapabilities,
				Log:              func(format string, v ...interface{}) {},
			}
			client := K8sDynamicClientFromCRs(tt.helmCRS...)
			clientInterface := k8sfake.NewSimpleClientset()
			coreClient := clientInterface.CoreV1()
			_, err := InstallChart("test-namespace", tt.releaseName, tt.chartPath, nil, actionConfig, &auth.User{}, tt.repositoryName, client, coreClient)
			fmt.Println("Error", err)
			if tt.testName == "valid chart path" {
				if err != nil {
					t.Error("Error occurred while installing chartPath")
				}
				rel, err := GetRelease(tt.releaseName, actionConfig)
				if err != nil {
					t.Error("Failed to get Release", rel)
				}
				if rel.Name != tt.releaseName {
					t.Error("Release name aren't matching")
				}
				if rel.Info.Status != release.StatusDeployed {
					t.Error("Chart isn't deployed")
				}
				if tt.manifestValue != rel.Manifest {
					t.Error("Manifest values aren't matching")
				}
			} else if tt.testName == "invalid chart path" {
				if err == nil {
					t.Error("Should fail to parse while locating invalid chart")
				}
			} else if tt.testName == "invalid release name" {
				rel, err := GetRelease(tt.releaseName, actionConfig)
				if rel != nil {
					t.Errorf("Release should be null %+v", rel)
				}
				if err == nil {
					t.Error("Error should be thrown in case no release found")
				}
			}
		})
	}
	err = ExecuteScript("./testdata/cleanupNonTls.sh")
	require.NoError(t, err)
}

func TestGetReleaseWithTlsData(t *testing.T) {
	os.Setenv("HELM_CLEANUP", "0")
	//create the server.key and server.crt
	err := ExecuteScript("./testdata/createTlsSecrets.sh")
	require.NoError(t, err)
	//start chartmuseum server
	err = ExecuteScript("./testdata/chartmuseum.sh")
	require.NoError(t, err)
	err = ExecuteScript("./testdata/cacertCreate.sh")
	require.NoError(t, err)
	err = ExecuteScript("./testdata/uploadCharts.sh")
	tests := []struct {
		releaseName     string
		chartPath       string
		chartName       string
		chartVersion    string
		createSecret    bool
		createNamespace bool
		createConfigMap bool
		namespace       string
		repoName        string
		helmCRS         []*unstructured.Unstructured
	}{
		{
			releaseName:     "my-release",
			chartPath:       "https://localhost:8443/charts/mychart-0.1.0.tgz",
			chartName:       "mychart",
			chartVersion:    "0.1.0",
			createSecret:    true,
			createNamespace: true,
			createConfigMap: true,
			namespace:       "test",
			repoName:        "my-repo",
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
	}

	for _, tt := range tests {
		t.Run(tt.releaseName, func(t *testing.T) {
			objs := []runtime.Object{}
			store := storage.Init(driver.NewMemory())
			actionConfig := &action.Configuration{
				RESTClientGetter: FakeConfig{},
				Releases:         store,
				KubeClient:       &kubefake.PrintingKubeClient{Out: ioutil.Discard},
				Capabilities:     chartutil.DefaultCapabilities,
				Log:              func(format string, v ...interface{}) {},
			}
			// create a namespace if it is not same as openshift-config
			if tt.createNamespace && tt.namespace != configNamespace {
				nsSpec := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: tt.namespace}}
				objs = append(objs, nsSpec)
			}
			// create a secret in required namespace
			if tt.createSecret {
				certificate, errCert := ioutil.ReadFile("./server.crt")
				require.NoError(t, errCert)
				key, errKey := ioutil.ReadFile("./server.key")
				require.NoError(t, errKey)
				data := map[string][]byte{
					"tls.key": key,
					"tls.crt": certificate,
				}
				if tt.namespace == "" {
					tt.namespace = configNamespace
				}
				secretSpec := &v1.Secret{Data: data, ObjectMeta: metav1.ObjectMeta{Name: "my-repo", Namespace: tt.namespace}}
				objs = append(objs, secretSpec)
			}
			//create a configMap in openshift-config namespace
			if tt.createConfigMap {
				caCert, err := ioutil.ReadFile("./cacert.pem")
				require.NoError(t, err)
				data := map[string]string{
					"ca-bundle.crt": string(caCert),
				}
				secretSpec := &v1.ConfigMap{Data: data, ObjectMeta: metav1.ObjectMeta{Name: "my-repo", Namespace: configNamespace}}
				objs = append(objs, secretSpec)
			}
			//client := fake.K8sDynamicClient("helm.openshift.io/v1beta1", "HelmChartRepository", "")
			//coreClient := k8sfake.NewSimpleClientset(objs...).CoreV1()
			client := K8sDynamicClientFromCRs(tt.helmCRS...)
			clientInterface := k8sfake.NewSimpleClientset(objs...)
			coreClient := clientInterface.CoreV1()
			_, err := InstallChart("test", tt.releaseName, tt.chartPath, nil, actionConfig, &auth.User{}, tt.repoName, client, coreClient)
			require.NoError(t, err)
			//require.Equal(t, tt.releaseName, rel.Name)
			rel, err := GetRelease(tt.releaseName, actionConfig)
			require.NoError(t, err)
			require.Equal(t, tt.releaseName, rel.Name)
		})
	}
	err = ExecuteScript("./testdata/cleanup.sh")
	require.NoError(t, err)
}
