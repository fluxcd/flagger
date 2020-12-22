package observers

import (
	"bufio"
	"bytes"
	"fmt"
	"text/template"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
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
