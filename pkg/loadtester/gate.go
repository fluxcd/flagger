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

package loadtester

import (
	"fmt"
	"sync"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

type GateStorage struct {
	backend string
	data    *sync.Map
}

func NewGateStorage(backend string) *GateStorage {
	return &GateStorage{
		backend: backend,
		data:    new(sync.Map),
	}
}

func (gs *GateStorage) open(key string) {
	gs.data.Store(key, true)
}

func (gs *GateStorage) close(key string) {
	gs.data.Store(key, false)
}

func (gs *GateStorage) isOpen(key string) (locked bool) {
	val, ok := gs.data.LoadOrStore(key, false)
	if ok {
		return val.(bool)
	}
	return
}

func gateKey(payload *flaggerv1.CanaryWebhookPayload) string {
	key := fmt.Sprintf("%s.%s", payload.Name, payload.Namespace)
	if payload.Checksum != "" {
		return fmt.Sprintf("%s.%s", key, payload.Checksum)
	}
	return key
}

func rollbackKey(payload *flaggerv1.CanaryWebhookPayload) string {
	return fmt.Sprintf("rollback.%s", gateKey(payload))
}
