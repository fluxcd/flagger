package factory

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/cache"

	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd/client/clientset/versioned"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"

	"github.com/solo-io/solo-kit/pkg/utils/stringutils"
	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/hashicorp/consul/api"
	vaultapi "github.com/hashicorp/vault/api"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/configmap"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/consul"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/file"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kube/crd"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/kubesecret"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/memory"
	"github.com/solo-io/solo-kit/pkg/api/v1/clients/vault"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources"
	"github.com/solo-io/solo-kit/pkg/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type NewResourceClientParams struct {
	ResourceType resources.Resource
	Token        string
}

// TODO(ilackarms): more opts validation
func newResourceClient(factory ResourceClientFactory, params NewResourceClientParams) (clients.ResourceClient, error) {
	resourceType := params.ResourceType
	switch opts := factory.(type) {
	case *KubeResourceClientFactory:
		kubeCfg := opts.Cfg
		if kubeCfg == nil {
			return nil, errors.Errorf("must provide a rest.Config for the kube resource client")
		}
		if params.Token != "" {
			kubeCfg.BearerToken = strings.TrimPrefix(params.Token, "Bearer ")
		}
		inputResource, ok := params.ResourceType.(resources.InputResource)
		if !ok {
			return nil, errors.Errorf("the kubernetes crd client can only be used for input resources, received type %v", resources.Kind(resourceType))
		}
		if opts.Crd.Type == nil {
			return nil, errors.Errorf("must provide a crd for the kube resource client")
		}
		if opts.SharedCache == nil {
			return nil, errors.Errorf("must provide a shared cache for the kube resource client")
		}

		// Validate namespace whitelist:
		// 1. If no namespace list was provided, default to all namespaces
		// 2. Error if namespace list contains the empty string plus other values
		namespaceWhitelist := opts.NamespaceWhitelist
		if len(namespaceWhitelist) == 0 {
			namespaceWhitelist = []string{metaV1.NamespaceAll}
		}
		if len(namespaceWhitelist) > 1 && stringutils.ContainsString(metaV1.NamespaceAll, namespaceWhitelist) {
			return nil, fmt.Errorf("the kube resource client namespace list must contain either "+
				"the empty string (all namespaces) or multiple non-empty strings. Found both: %v", namespaceWhitelist)
		}

		// If the flag is false, call the k8s apiext API to create the given CRD.
		if !opts.SkipCrdCreation {
			apiExts, err := clientset.NewForConfig(kubeCfg)
			if err != nil {
				return nil, errors.Wrapf(err, "creating api extensions client")
			}
			if err := opts.Crd.Register(apiExts); err != nil {
				return nil, err
			}
		}

		// Create clientset for solo resources
		crdClient, err := versioned.NewForConfig(kubeCfg, opts.Crd)
		if err != nil {
			return nil, errors.Wrapf(err, "creating crd client")
		}

		return kube.NewResourceClient(
			opts.Crd,
			crdClient,
			opts.SharedCache,
			inputResource,
			namespaceWhitelist,
			opts.ResyncPeriod,
		), nil

	case *ConsulResourceClientFactory:
		return consul.NewResourceClient(opts.Consul, opts.RootKey, resourceType), nil
	case *FileResourceClientFactory:
		return file.NewResourceClient(opts.RootDir, resourceType), nil
	case *MemoryResourceClientFactory:
		return memory.NewResourceClient(opts.Cache, resourceType), nil
	case *KubeConfigMapClientFactory:
		if opts.Cache == nil {
			return nil, errors.Errorf("invalid opts, configmap client requires a kube core cache")
		}
		if opts.CustomtConverter != nil {
			return configmap.NewResourceClientWithConverter(opts.Clientset, resourceType, opts.Cache, opts.CustomtConverter)
		}
		return configmap.NewResourceClient(opts.Clientset, resourceType, opts.Cache, opts.PlainConfigmaps)
	case *KubeSecretClientFactory:
		if opts.Cache == nil {
			return nil, errors.Errorf("invalid opts, secret client requires a kube core cache")
		}
		if opts.SecretConverter != nil {
			return kubesecret.NewResourceClientWithSecretConverter(opts.Clientset, resourceType, opts.Cache, opts.SecretConverter)
		}
		return kubesecret.NewResourceClient(opts.Clientset, resourceType, opts.PlainSecrets, opts.Cache)
	case *VaultSecretClientFactory:
		return vault.NewResourceClient(opts.Vault, opts.RootKey, resourceType), nil
	}
	panic("unsupported type " + reflect.TypeOf(factory).Name())
}

// https://golang.org/doc/faq#generics
type ResourceClientFactory interface {
	NewResourceClient(params NewResourceClientParams) (clients.ResourceClient, error)
}

// If SkipCrdCreation is set to 'true', the clients built with this factory will not attempt to create the given CRD
// during registration. This allows us to create and register resource clients in cases where the given configuration
// contains a token associated with a user that is not authorized to create CRDs.
// Clients built with this factory will be able to access only resources the given namespace list. If no value is provided,
// clients will be able to access resources in all namespaces.
type KubeResourceClientFactory struct {
	Crd                crd.Crd
	Cfg                *rest.Config
	SharedCache        kube.SharedCache
	SkipCrdCreation    bool
	NamespaceWhitelist []string
	ResyncPeriod       time.Duration
}

func (f *KubeResourceClientFactory) NewResourceClient(params NewResourceClientParams) (clients.ResourceClient, error) {
	return newResourceClient(f, params)
}

type ConsulResourceClientFactory struct {
	Consul  *api.Client
	RootKey string
}

func (f *ConsulResourceClientFactory) NewResourceClient(params NewResourceClientParams) (clients.ResourceClient, error) {
	return newResourceClient(f, params)
}

type FileResourceClientFactory struct {
	RootDir string
}

func (f *FileResourceClientFactory) NewResourceClient(params NewResourceClientParams) (clients.ResourceClient, error) {
	return newResourceClient(f, params)
}

type MemoryResourceClientFactory struct {
	Cache memory.InMemoryResourceCache
}

func (f *MemoryResourceClientFactory) NewResourceClient(params NewResourceClientParams) (clients.ResourceClient, error) {
	return newResourceClient(f, params)
}

type KubeConfigMapClientFactory struct {
	Clientset kubernetes.Interface
	Cache     cache.KubeCoreCache
	// set this  to true if resource fields are all strings
	// resources will be stored as plain kubernetes configmaps without serializing/deserializing as objects
	PlainConfigmaps bool
	// a custom handler to define how configmaps are serialized/deserialized out of resources
	// if set, Plain is ignored
	CustomtConverter configmap.ConfigMapConverter
}

func (f *KubeConfigMapClientFactory) NewResourceClient(params NewResourceClientParams) (clients.ResourceClient, error) {
	return newResourceClient(f, params)
}

type KubeSecretClientFactory struct {
	Clientset kubernetes.Interface
	// set this  to true if resource fields are all strings
	// resources will be stored as plain kubernetes secrets without serializing/deserializing as objects
	PlainSecrets    bool
	SecretConverter kubesecret.SecretConverter
	Cache           cache.KubeCoreCache
}

func (f *KubeSecretClientFactory) NewResourceClient(params NewResourceClientParams) (clients.ResourceClient, error) {
	return newResourceClient(f, params)
}

type VaultSecretClientFactory struct {
	Vault   *vaultapi.Client
	RootKey string
}

func (f *VaultSecretClientFactory) NewResourceClient(params NewResourceClientParams) (clients.ResourceClient, error) {
	return newResourceClient(f, params)
}
