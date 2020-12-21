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
	"fmt"
	"regexp"
	"time"

	flaggerv1 "github.com/fluxcd/flagger/pkg/apis/flagger/v1beta1"
	"github.com/fluxcd/flagger/pkg/logger"

	"github.com/fluxcd/flagger/pkg/metrics/providers"
)

const routePattern = `{{- $route := printf "kube(ew)?_%s__%s_canary__.*__%s_canary(_[0-9]+)?" namespace ingress service }}`

var skipperQueries = map[string]string{
	"request-success-rate": routePattern + `
	sum(rate(skipper_response_duration_seconds_bucket{route=~"{{ $route }}",code!~"5..",le="+Inf"}[{{ interval }}])) / 
	sum(rate(skipper_response_duration_seconds_bucket{route=~"{{ $route }}",le="+Inf"}[{{ interval }}])) * 100`,
	"request-duration": routePattern + `
	sum(rate(skipper_serve_route_duration_seconds_sum{route=~"{{ $route }}"}[{{ interval }}])) / 
	sum(rate(skipper_serve_route_duration_seconds_count{route=~"{{ $route }}"}[{{ interval }}])) * 1000`,
}

// SkipperObserver Implementation for Skipper (https://github.com/zalando/skipper)
type SkipperObserver struct {
	client providers.Interface
}

// GetRequestSuccessRate return value for Skipper Request Success Rate
func (ob *SkipperObserver) GetRequestSuccessRate(model flaggerv1.MetricTemplateModel) (float64, error) {

	model = encodeModelForSkipper(model)

	query, err := RenderQuery(skipperQueries["request-success-rate"], model)
	if err != nil {
		return 0, fmt.Errorf("rendering query failed: %w", err)
	}
	logger, _ := logger.NewLoggerWithEncoding("debug", "json")
	logger.Debugf("GetRequestSuccessRate: %s", query)

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, fmt.Errorf("running query failed: %w", err)
	}

	return value, nil
}

// GetRequestDuration return value for Skipper Request Duration
func (ob *SkipperObserver) GetRequestDuration(model flaggerv1.MetricTemplateModel) (time.Duration, error) {

	model = encodeModelForSkipper(model)

	query, err := RenderQuery(skipperQueries["request-duration"], model)
	if err != nil {
		return 0, fmt.Errorf("rendering query failed: %w", err)
	}
	logger, _ := logger.NewLoggerWithEncoding("debug", "json")
	logger.Debugf("GetRequestDuration: %s", query)

	value, err := ob.client.RunQuery(query)
	if err != nil {
		return 0, fmt.Errorf("running query failed: %w", err)
	}

	ms := time.Duration(int64(value)) * time.Millisecond
	return ms, nil
}

// encodeModelForSkipper replaces non word character in model with underscore to match route names
// https://github.com/zalando/skipper/blob/dd70bd65e7f99cfb5dd6b6f71885d9fe3b2707f6/dataclients/kubernetes/ingress.go#L101
func encodeModelForSkipper(model flaggerv1.MetricTemplateModel) flaggerv1.MetricTemplateModel {
	nonWord := regexp.MustCompile(`\W`)
	model.Ingress = nonWord.ReplaceAllString(model.Ingress, "_")
	model.Name = nonWord.ReplaceAllString(model.Name, "_")
	model.Namespace = nonWord.ReplaceAllString(model.Namespace, "_")
	model.Service = nonWord.ReplaceAllString(model.Service, "_")
	model.Target = nonWord.ReplaceAllString(model.Target, "_")

	return model
}
