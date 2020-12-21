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

package observers

import (
	"bufio"
	"bytes"
	"fmt"
	"text/template"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
)

func RenderQuery(queryTemplate string, model flaggerv1.MetricTemplateModel) (string, error) {
	t, err := template.New("tmpl").Funcs(model.TemplateFunctions()).Parse(queryTemplate)
	if err != nil {
		return "", fmt.Errorf("template parsing failed: %w", err)
	}
	var data bytes.Buffer
	b := bufio.NewWriter(&data)

	if err := t.Execute(b, nil); err != nil {
		return "", fmt.Errorf("template excution failed: %w", err)
	}

	err = b.Flush()
	if err != nil {
		return "", fmt.Errorf("buffer flush failed: %w", err)
	}
	return data.String(), nil
}
