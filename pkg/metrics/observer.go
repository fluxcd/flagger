package metrics

import (
	"time"
)

type Interface interface {
	GetRequestSuccessRate(name string, namespace string, interval string) (float64, error)
	GetRequestDuration(name string, namespace string, interval string) (time.Duration, error)
}
