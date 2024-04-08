package providers

import (
	"context"
	"errors"
	"fmt"
	"k8s.io/klog/v2"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// api version for the Keptn Metric CRDs
const (
	apiVersion = "v1beta1"

	groupName                = "metrics.keptn.sh"
	keptnMetricsResourceName = "keptnmetrics"
	analysisResourceName     = "analyses"
)

var keptnMetricsResource = schema.GroupVersionResource{
	Group:    groupName,
	Version:  apiVersion,
	Resource: keptnMetricsResourceName,
}

var analysisResource = schema.GroupVersionResource{
	Group:    groupName,
	Version:  apiVersion,
	Resource: analysisResourceName,
}

type queryObject struct {
	GroupVersionResource schema.GroupVersionResource
	ResourceName         string
	DurationString       string
	Namespace            string
	Arguments            map[string]interface{}
}

type KeptnProvider struct {
	client          dynamic.Interface
	analysisTimeout time.Duration
}

func NewKeptnProvider(cfg *rest.Config) (*KeptnProvider, error) {
	if cfg == nil {
		return nil, errors.New("could not initialize KeptnProvider: no KubeConfig provided")
	}
	client, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("could not initialize KeptnProvider: %w", err)
	}
	return &KeptnProvider{
		client:          client,
		analysisTimeout: 10 * time.Second,
	}, nil
}

// RunQuery fetches the value of a KeptnMetric or Analysis,
// based on the selector provided in the query.
// The format of the selector is the following:
// <keptnmetric|analysis>/<namespace>/<resourceName>/<duration>/<arguments>
func (k *KeptnProvider) RunQuery(query string) (float64, error) {
	queryObj, err := parseQuery(query)
	if err != nil {
		return 0, err
	}

	switch queryObj.GroupVersionResource.Resource {
	case keptnMetricsResourceName:
		return k.queryKeptnMetric(queryObj)
	case analysisResourceName:
		return k.queryKeptnAnalysis(queryObj)
	default:
		return 0, errors.New("unsupported query")
	}

}

func (k *KeptnProvider) IsOnline() (bool, error) {
	// TODO should we check for the keptn deployment to be up and running in the cluster?
	return true, nil
}

func (k *KeptnProvider) queryKeptnMetric(queryObj *queryObject) (float64, error) {
	get, err := k.client.Resource(queryObj.GroupVersionResource).
		Namespace(queryObj.Namespace).
		Get(
			context.Background(),
			queryObj.ResourceName,
			v1.GetOptions{},
		)

	if err != nil {
		return 0, fmt.Errorf("could not retrieve KeptnMetric %s/%s: %w", queryObj.Namespace, queryObj.ResourceName, err)
	}

	if status, ok := get.Object["status"]; ok {
		if statusObj, ok := status.(map[string]interface{}); ok {
			if value, ok := statusObj["value"].(string); ok {
				floatValue, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return 0, fmt.Errorf("could not parse value of KeptnMetric %s/%s to float: %w", queryObj.Namespace, queryObj.ResourceName, err)
				}
				return floatValue, nil
			}
		}
	}
	return 0, fmt.Errorf("could not retrieve KeptnMetric - no value found in resource %s/%s", queryObj.Namespace, queryObj.ResourceName)
}

func (k *KeptnProvider) queryKeptnAnalysis(obj *queryObject) (float64, error) {
	analysis := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": fmt.Sprintf("metrics.keptn.sh/%s", apiVersion),
			"kind":       "Analysis",
			"metadata": map[string]interface{}{
				"name":      fmt.Sprintf("%s-%s", obj.ResourceName, uuid.New().String()[:6]),
				"namespace": obj.Namespace,
			},
			"spec": map[string]interface{}{
				"analysisDefinition": map[string]interface{}{
					"name": obj.ResourceName,
				},
				"timeframe": map[string]interface{}{
					"recent": obj.DurationString,
				},
				"args": obj.Arguments,
			},
		},
	}

	// set the timeout to 10s - this will give Keptn enough time to reconcile the Analysis
	// and store the result in the status of the resource created here.
	ctx, cancel := context.WithTimeout(context.Background(), k.analysisTimeout)
	defer cancel()

	createdAnalysis, err := k.client.
		Resource(obj.GroupVersionResource).
		Namespace(obj.Namespace).
		Create(ctx, analysis, v1.CreateOptions{})

	if err != nil {
		return 0, fmt.Errorf("could not create Keptn Analysis %s/%s: %w", obj.Namespace, obj.ResourceName, err)
	}

	// delete the created analysis at the end of the function
	defer func() {
		err := k.client.
			Resource(obj.GroupVersionResource).
			Namespace(obj.Namespace).
			Delete(
				context.TODO(),
				createdAnalysis.GetName(),
				v1.DeleteOptions{},
			)
		if err != nil {
			klog.Errorf("Could not delete Keptn Analysis '%s': %v", createdAnalysis.GetName(), err)
		}
	}()

	for {
		// retrieve the current state of the created Analysis resource every 1s, until
		// it has been completed, and the evaluation result is available.
		// We do this until the timeout of the context expires. If no result is available
		// by then, we return an error.
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("encountered timeout while waiting for Keptn Analysis %s/%s to be finished", obj.Namespace, obj.ResourceName)
		case <-time.After(time.Second):
			get, err := k.client.Resource(obj.GroupVersionResource).Namespace(obj.Namespace).Get(ctx, createdAnalysis.GetName(), v1.GetOptions{})
			if err != nil {
				return 0, fmt.Errorf("could not check status of created Keptn Analysis %s/%s: %w", obj.Namespace, obj.ResourceName, err)
			}
			statusStr, ok, err := unstructured.NestedString(get.Object, "status", "state")
			if err != nil {
				return 0, fmt.Errorf("could not check status of created Keptn Analysis %s/%s: %w", obj.Namespace, obj.ResourceName, err)
			}
			if ok && statusStr == "Completed" {
				passed, ok, err := unstructured.NestedBool(get.Object, "status", "pass")
				if err != nil {
					return 0, fmt.Errorf("could not check status of created Keptn Analysis %s/%s: %w", obj.Namespace, obj.ResourceName, err)
				}
				if ok {
					if passed {
						return 1, nil
					}
					return 0, nil
				}
			}
		}
	}

}

func parseQuery(query string) (*queryObject, error) {
	result := &queryObject{}
	// sanitize the query by converting to lower case, trimming spaces and line break characters
	split := strings.Split(
		strings.TrimSpace(
			strings.TrimSuffix(
				strings.ToLower(query),
				"\n",
			),
		),
		"/",
	)

	if len(split) < 3 {
		return nil, errors.New("unexpected query format. query must be in the format <keptnmetric|analysis>/<namespace>/<resourceName>/<duration>/<arguments>")
	}
	switch split[0] {
	// take into account both singular and plural naming of resource names, to reduce probability of errors
	case "keptnmetric", keptnMetricsResourceName:
		result.GroupVersionResource = keptnMetricsResource
	case "analysis", analysisResourceName:
		result.GroupVersionResource = analysisResource
		// add the duration for the Analysis, if available
		if len(split) >= 4 {
			result.DurationString = split[3]
		} else {
			//set to '1m' by default
			result.DurationString = "1m"
		}

		// add arguments - these are provided as a comma separated list of key/value pairs
		result.Arguments = map[string]interface{}{}
		if len(split) >= 5 {
			args := strings.Split(split[4], ";")

			for i := 0; i < len(args); i++ {
				keyValue := strings.Split(args[i], "=")
				if len(keyValue) == 2 {
					result.Arguments[keyValue[0]] = keyValue[1]
				}
			}
		}

	default:
		return nil, errors.New("unexpected resource kind provided in the query. must be one of: ['keptnmetric', 'analysis']")
	}

	result.Namespace = split[1]
	result.ResourceName = split[2]

	return result, nil
}
