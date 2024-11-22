/*
Copyright 2024 The KEDA Authors.

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

package v1alpha1

import (
	"fmt"
)

var (
	interceptorImage = fmt.Sprintf("ghcr.io/kedacore/http-add-on-interceptor:%s", Version())
	scalerImage      = fmt.Sprintf("ghcr.io/kedacore/http-add-on-scaler:%s", Version())
)

func (c *HTTPInterceptorSpec) GetProxyPort() int32 {
	if c.Config == nil || c.Config.ProxyPort == nil {
		return 8080
	}
	return *c.Config.ProxyPort
}

func (c *HTTPInterceptorSpec) GetAdminPort() int32 {
	if c.Config == nil || c.Config.AdminPort == nil {
		return 9090
	}
	return *c.Config.AdminPort
}
func (c *HTTPInterceptorSpec) GetConnectTimeout() string {
	if c.Config == nil || c.Config.ConnectTimeout == nil {
		return "500ms"
	}
	return *c.Config.ConnectTimeout
}
func (c *HTTPInterceptorSpec) GetHeaderTimeout() string {
	if c.Config == nil || c.Config.HeaderTimeout == nil {
		return "500ms"
	}
	return *c.Config.HeaderTimeout
}
func (c *HTTPInterceptorSpec) GetWaitTimeout() string {
	if c.Config == nil || c.Config.WaitTimeout == nil {
		return "1500ms"
	}
	return *c.Config.WaitTimeout
}
func (c *HTTPInterceptorSpec) GetIdleConnTimeout() string {
	if c.Config == nil || c.Config.IdleConnTimeout == nil {
		return "90s"
	}
	return *c.Config.IdleConnTimeout
}
func (c *HTTPInterceptorSpec) GetTLSHandshakeTimeout() string {
	if c.Config == nil || c.Config.TLSHandshakeTimeout == nil {
		return "10s"
	}
	return *c.Config.TLSHandshakeTimeout
}
func (c *HTTPInterceptorSpec) GetExpectContinueTimeout() string {
	if c.Config == nil || c.Config.ExpectContinueTimeout == nil {
		return "1s"
	}
	return *c.Config.ExpectContinueTimeout
}
func (c *HTTPInterceptorSpec) GetForceHTTP2() bool {
	if c.Config == nil || c.Config.ForceHTTP2 == nil {
		return false
	}
	return *c.Config.ForceHTTP2
}
func (c *HTTPInterceptorSpec) GetKeepAlive() string {
	if c.Config == nil || c.Config.KeepAlive == nil {
		return "1s"
	}
	return *c.Config.KeepAlive
}
func (c *HTTPInterceptorSpec) GetMaxIdleConns() int {
	if c.Config == nil || c.Config.MaxIdleConns == nil {
		return 100
	}
	return *c.Config.MaxIdleConns
}
func (c *HTTPInterceptorSpec) GetPollingInterval() int {
	if c.Config == nil || c.Config.PollingInterval == nil {
		return 1000
	}
	return *c.Config.PollingInterval
}

func (c *HTTPInterceptorSpec) GetImage() string {
	if c.Image == nil {
		return interceptorImage
	}
	return *c.Image
}

func (c *HTTPInterceptorSpec) GetLabels() map[string]string {
	if c.Labels == nil {
		return map[string]string{}
	}
	return c.Labels
}

func (c *HTTPInterceptorSpec) GetAnnotations() map[string]string {
	if c.Annotations == nil {
		return map[string]string{}
	}
	return c.Annotations
}

func (c *HTTPScalerSpec) GetPort() int32 {
	if c.Config.Port == nil {
		return 9090
	}
	return *c.Config.Port
}

func (c *HTTPScalerSpec) GetImage() string {
	if c.Image == nil {
		return scalerImage
	}
	return *c.Image
}

func (c *HTTPScalerSpec) GetLabels() map[string]string {
	if c.Labels == nil {
		return map[string]string{}
	}
	return c.Labels
}

func (c *HTTPScalerSpec) GetAnnotations() map[string]string {
	if c.Annotations == nil {
		return map[string]string{}
	}
	return c.Annotations
}
