package observers

import (
	"time"

	flaggerv1 "github.com/weaveworks/flagger/pkg/apis/flagger/v1beta1"
)

type Interface interface {
	GetRequestSuccessRate(model flaggerv1.MetricTemplateModel) (float64, error)
	GetRequestDuration(model flaggerv1.MetricTemplateModel) (time.Duration, error)
}
