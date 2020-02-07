package providers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/weaveworks/flagger/pkg/client/clientset/versioned"
	fakeFlagger "github.com/weaveworks/flagger/pkg/client/clientset/versioned/fake"
)

type fakeClients struct {
	kubeClient    kubernetes.Interface
	flaggerClient clientset.Interface
}

func prometheusFake() fakeClients {
	provider := flaggerv1.MetricTemplateProvider{
		Type:      "prometheus",
		Address:   "http://prometheus:9090",
		SecretRef: &corev1.LocalObjectReference{Name: "prometheus"},
	}

	template := &flaggerv1.MetricTemplate{
		TypeMeta: metav1.TypeMeta{APIVersion: flaggerv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "prometheus",
		},
		Spec: flaggerv1.MetricTemplateSpec{
			Provider: provider,
			Query:    "sum(envoy_cluster_upstream_rq)",
		},
	}

	flaggerClient := fakeFlagger.NewSimpleClientset(template)

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "prometheus",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"username": []byte("username"),
			"password": []byte("password"),
		},
	}

	kubeClient := fake.NewSimpleClientset(secret)

	return fakeClients{
		kubeClient:    kubeClient,
		flaggerClient: flaggerClient,
	}
}

func TestNewPrometheusProvider(t *testing.T) {
	clients := prometheusFake()

	template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get("prometheus", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	secret, err := clients.kubeClient.CoreV1().Secrets("default").Get("prometheus", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	prom, err := NewPrometheusProvider(template.Spec.Provider, secret.Data)
	if err != nil {
		t.Fatal(err.Error())
	}

	if prom.url.String() != "http://prometheus:9090" {
		t.Errorf("Got URL %s wanted %s", prom.url.String(), "http://prometheus:9090")
	}

	if prom.password != "password" {
		t.Errorf("Got password %s wanted %s", prom.password, "password")
	}
}

func TestPrometheusProvider_RunQueryWithBasicAuth(t *testing.T) {
	expected := `sum(envoy_cluster_upstream_rq)`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		promql := r.URL.Query()["query"][0]
		if promql != expected {
			t.Errorf("\nGot %s \nWanted %s", promql, expected)
		}

		if header, ok := r.Header["Authorization"]; ok {
			if !strings.Contains(header[0], "Basic") {
				t.Error("Basic authorization header not found")
			}
		} else {
			t.Error("Authorization header not found")
		}

		json := `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1545905245.458,"100"]}]}}`
		w.Write([]byte(json))
	}))
	defer ts.Close()

	clients := prometheusFake()

	template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get("prometheus", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	template.Spec.Provider.Address = ts.URL

	secret, err := clients.kubeClient.CoreV1().Secrets("default").Get("prometheus", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}

	prom, err := NewPrometheusProvider(template.Spec.Provider, secret.Data)
	if err != nil {
		t.Fatal(err.Error())
	}

	val, err := prom.RunQuery(template.Spec.Query)
	if err != nil {
		t.Fatal(err.Error())
	}

	if val != 100 {
		t.Errorf("Got %v wanted %v", val, 100)
	}
}

func TestPrometheusProvider_IsOnline(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()

	clients := prometheusFake()

	template, err := clients.flaggerClient.FlaggerV1beta1().MetricTemplates("default").Get("prometheus", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err.Error())
	}
	template.Spec.Provider.Address = ts.URL
	template.Spec.Provider.SecretRef = nil

	prom, err := NewPrometheusProvider(template.Spec.Provider, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	ok, err := prom.IsOnline()
	if err == nil {
		t.Errorf("Got no error wanted %v", http.StatusBadGateway)
	}

	if ok {
		t.Errorf("Got %v wanted %v", ok, false)
	}
}
