/*
Copyright 2020 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package router

import (
	"strings"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	clientset "github.com/fluxcd/flagger/pkg/client/clientset/versioned"
	knative "knative.dev/serving/pkg/client/clientset/versioned"
)

type Factory struct {
	kubeConfig               *restclient.Config
	kubeClient               kubernetes.Interface
	meshClient               clientset.Interface
	flaggerClient            clientset.Interface
	knativeClient            knative.Interface
	ingressAnnotationsPrefix string
	ingressClass             string
	logger                   *zap.SugaredLogger
	setOwnerRefs             bool
}

func NewFactory(kubeConfig *restclient.Config, kubeClient kubernetes.Interface,
	flaggerClient clientset.Interface,
	knativeClient knative.Interface,
	ingressAnnotationsPrefix string,
	ingressClass string,
	logger *zap.SugaredLogger,
	meshClient clientset.Interface,
	setOwnerRefs bool) *Factory {
	return &Factory{
		kubeConfig:               kubeConfig,
		meshClient:               meshClient,
		kubeClient:               kubeClient,
		flaggerClient:            flaggerClient,
		knativeClient:            knativeClient,
		ingressAnnotationsPrefix: ingressAnnotationsPrefix,
		ingressClass:             ingressClass,
		logger:                   logger,
		setOwnerRefs:             setOwnerRefs,
	}
}

// KubernetesRouter returns a KubernetesRouter interface implementation
func (factory *Factory) KubernetesRouter(kind string, labelSelector string, labelValue string, ports map[string]int32) KubernetesRouter {
	switch kind {
	case "Service":
		return &KubernetesNoopRouter{}
	default: // Daemonset or Deployment
		return &KubernetesDefaultRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			labelSelector: labelSelector,
			labelValue:    labelValue,
			ports:         ports,
		}
	}
}

// MeshRouter returns a service mesh router
func (factory *Factory) MeshRouter(provider string, labelSelector string) Interface {
	switch {
	case strings.HasPrefix(provider, flaggerv1.AppMeshProvider+":v1beta2"):
		return &AppMeshv1beta2Router{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			appmeshClient: factory.meshClient,
			labelSelector: labelSelector,
			setOwnerRefs:  factory.setOwnerRefs,
		}
	case provider == flaggerv1.AppMeshProvider:
		return &AppMeshRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			appmeshClient: factory.meshClient,
			setOwnerRefs:  factory.setOwnerRefs,
		}
	case provider == flaggerv1.LinkerdProvider:
		return &SmiRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			smiClient:     factory.meshClient,
			targetMesh:    flaggerv1.LinkerdProvider,
			setOwnerRefs:  factory.setOwnerRefs,
		}
	case provider == flaggerv1.IstioProvider:
		return &IstioRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			istioClient:   factory.meshClient,
			setOwnerRefs:  factory.setOwnerRefs,
		}
	case strings.HasPrefix(provider, flaggerv1.SMIProvider+":v1alpha1"):
		mesh := strings.TrimPrefix(provider, flaggerv1.SMIProvider+":v1alpha1:")
		return &SmiRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			smiClient:     factory.meshClient,
			targetMesh:    mesh,
			setOwnerRefs:  factory.setOwnerRefs,
		}
	case strings.HasPrefix(provider, flaggerv1.SMIProvider+":v1alpha2"):
		mesh := strings.TrimPrefix(provider, flaggerv1.SMIProvider+":v1alpha2:")
		return &Smiv1alpha2Router{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			smiClient:     factory.meshClient,
			targetMesh:    mesh,
			setOwnerRefs:  factory.setOwnerRefs,
		}
	case strings.HasPrefix(provider, flaggerv1.SMIProvider+":v1alpha3"):
		mesh := strings.TrimPrefix(provider, flaggerv1.SMIProvider+":v1alpha3:")
		return &Smiv1alpha3Router{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			smiClient:     factory.meshClient,
			targetMesh:    mesh,
			setOwnerRefs:  factory.setOwnerRefs,
		}
	case provider == flaggerv1.ContourProvider:
		return &ContourRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			contourClient: factory.meshClient,
			ingressClass:  factory.ingressClass,
			setOwnerRefs:  factory.setOwnerRefs,
		}
	case provider == flaggerv1.KnativeProvider:
		return &KnativeRouter{
			knativeClient: factory.knativeClient,
		}
	case strings.HasPrefix(provider, flaggerv1.GlooProvider):
		return &GlooRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			glooClient:    factory.meshClient,
			setOwnerRefs:  factory.setOwnerRefs,
		}
	case provider == flaggerv1.NGINXProvider:
		return &IngressRouter{
			logger:            factory.logger,
			kubeClient:        factory.kubeClient,
			annotationsPrefix: factory.ingressAnnotationsPrefix,
			setOwnerRefs:      factory.setOwnerRefs,
		}
	case provider == flaggerv1.SkipperProvider:
		return &SkipperRouter{
			logger:       factory.logger,
			kubeClient:   factory.kubeClient,
			setOwnerRefs: factory.setOwnerRefs,
		}
	case provider == flaggerv1.TraefikProvider:
		return &TraefikRouter{
			logger:        factory.logger,
			traefikClient: factory.meshClient,
			setOwnerRefs:  factory.setOwnerRefs,
		}
	case provider == flaggerv1.ApisixProvider:
		return &ApisixRouter{
			logger:       factory.logger,
			apisixClient: factory.meshClient,
			setOwnerRefs: factory.setOwnerRefs,
		}
	case provider == flaggerv1.OsmProvider:
		return &Smiv1alpha2Router{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			smiClient:     factory.meshClient,
			targetMesh:    flaggerv1.OsmProvider,
			setOwnerRefs:  factory.setOwnerRefs,
		}
	case provider == flaggerv1.KumaProvider:
		return &KumaRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			kumaClient:    factory.meshClient,
		}
	case strings.HasPrefix(provider, flaggerv1.GatewayAPIProvider+":v1beta1"):
		return &GatewayAPIV1Beta1Router{
			logger:           factory.logger,
			kubeClient:       factory.kubeClient,
			gatewayAPIClient: factory.meshClient,
			setOwnerRefs:     factory.setOwnerRefs,
		}
	case strings.HasPrefix(provider, flaggerv1.GatewayAPIProvider+":v1"):
		return &GatewayAPIRouter{
			logger:           factory.logger,
			kubeClient:       factory.kubeClient,
			gatewayAPIClient: factory.meshClient,
			setOwnerRefs:     factory.setOwnerRefs,
		}
	case provider == flaggerv1.KubernetesProvider:
		return &NopRouter{}
	default:
		return &IstioRouter{
			logger:        factory.logger,
			flaggerClient: factory.flaggerClient,
			kubeClient:    factory.kubeClient,
			istioClient:   factory.meshClient,
			setOwnerRefs:  factory.setOwnerRefs,
		}
	}
}
