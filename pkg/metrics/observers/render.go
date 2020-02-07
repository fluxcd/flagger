package observers

import (
	"bufio"
	"bytes"
	"text/template"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

func RenderQuery(queryTemplate string, model flaggerv1.MetricTemplateModel) (string, error) {
	t, err := template.New("tmpl").Funcs(model.TemplateFunctions()).Parse(queryTemplate)
	if err != nil {
		return "", err
	}
	var data bytes.Buffer
	b := bufio.NewWriter(&data)

	if err := t.Execute(b, nil); err != nil {
		return "", err
	}

	err = b.Flush()
	if err != nil {
		return "", err
	}

	return data.String(), nil
}
