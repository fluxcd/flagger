// generated from Dynatraces DQL Query openapi.yaml version 1.17.0 on 2026-02-19
// by github.com/oapi-codegen/oapi-codegen/v2 version v2.0.0-00010101000000-000000000000
// everything but query:execute, query:poll, and query:verify removed
package providers

import (
	"encoding/json"
	"time"
)



// Defines values for FieldTypeType.
const (
	FieldTypeTypeArray        FieldTypeType = "array"
	FieldTypeTypeBinary       FieldTypeType = "binary"
	FieldTypeTypeBoolean      FieldTypeType = "boolean"
	FieldTypeTypeDouble       FieldTypeType = "double"
	FieldTypeTypeDuration     FieldTypeType = "duration"
	FieldTypeTypeGeoPoint     FieldTypeType = "geo_point"
	FieldTypeTypeIpAddress    FieldTypeType = "ip_address"
	FieldTypeTypeLong         FieldTypeType = "long"
	FieldTypeTypeRecord       FieldTypeType = "record"
	FieldTypeTypeSmartscapeId FieldTypeType = "smartscape_id"
	FieldTypeTypeString       FieldTypeType = "string"
	FieldTypeTypeTimeframe    FieldTypeType = "timeframe"
	FieldTypeTypeTimestamp    FieldTypeType = "timestamp"
	FieldTypeTypeUid          FieldTypeType = "uid"
	FieldTypeTypeUndefined    FieldTypeType = "undefined"
)

// Valid indicates whether the value is a known member of the FieldTypeType enum.
func (e FieldTypeType) Valid() bool {
	switch e {
	case FieldTypeTypeArray:
		return true
	case FieldTypeTypeBinary:
		return true
	case FieldTypeTypeBoolean:
		return true
	case FieldTypeTypeDouble:
		return true
	case FieldTypeTypeDuration:
		return true
	case FieldTypeTypeGeoPoint:
		return true
	case FieldTypeTypeIpAddress:
		return true
	case FieldTypeTypeLong:
		return true
	case FieldTypeTypeRecord:
		return true
	case FieldTypeTypeSmartscapeId:
		return true
	case FieldTypeTypeString:
		return true
	case FieldTypeTypeTimeframe:
		return true
	case FieldTypeTypeTimestamp:
		return true
	case FieldTypeTypeUid:
		return true
	case FieldTypeTypeUndefined:
		return true
	default:
		return false
	}
}

// Defines values for QueryState.
const (
	CANCELLED  QueryState = "CANCELLED"
	FAILED     QueryState = "FAILED"
	NOTSTARTED QueryState = "NOT_STARTED"
	RESULTGONE QueryState = "RESULT_GONE"
	RUNNING    QueryState = "RUNNING"
	SUCCEEDED  QueryState = "SUCCEEDED"
)

// Valid indicates whether the value is a known member of the QueryState enum.
func (e QueryState) Valid() bool {
	switch e {
	case CANCELLED:
		return true
	case FAILED:
		return true
	case NOTSTARTED:
		return true
	case RESULTGONE:
		return true
	case RUNNING:
		return true
	case SUCCEEDED:
		return true
	default:
		return false
	}
}

// BucketContribution defines model for BucketContribution.
type BucketContribution struct {
	MatchedRecordsRatio *float64 `json:"matchedRecordsRatio,omitempty"`
	Name                *string  `json:"name,omitempty"`
	ScannedBytes        *int64   `json:"scannedBytes,omitempty"`
	Table               *string  `json:"table,omitempty"`
}

// Contributions defines model for Contributions.
type Contributions struct {
	Buckets *[]BucketContribution `json:"buckets,omitempty"`
}


// ErrorEnvelope An 'envelope' error object that has the mandatory error object.
type ErrorEnvelope struct {
	// Error The response for error state
	Error ErrorResponse `json:"error"`
}

// ErrorResponse The response for error state
type ErrorResponse struct {
	// Code Error code, which normally matches the HTTP error code.
	Code int32 `json:"code"`

	// Details Detailed information about the error.
	Details *ErrorResponseDetails `json:"details,omitempty"`

	// Message A short, clear error message without details
	Message string `json:"message"`

	// RetryAfterSeconds The number of seconds to wait until the next retry.
	RetryAfterSeconds *int32 `json:"retryAfterSeconds,omitempty"`
}

// ErrorResponseDetails Detailed information about the error.
type ErrorResponseDetails struct {
	// Arguments The arguments for the message format.
	Arguments *[]string `json:"arguments,omitempty"`

	// ConstraintViolations Information about an input parameter that violated some validation rule of the service API.
	ConstraintViolations *[]struct {
		// Message Message describing the error.
		Message string `json:"message"`

		// ParameterDescriptor Describes the violating parameter.
		ParameterDescriptor *string `json:"parameterDescriptor,omitempty"`

		// ParameterLocation Describes the general location of the violating parameter.
		ParameterLocation *string `json:"parameterLocation,omitempty"`
	} `json:"constraintViolations,omitempty"`

	// ErrorMessage Complete error message.
	ErrorMessage *string `json:"errorMessage,omitempty"`

	// ErrorMessageFormat The message format of the error message, string.format based.
	ErrorMessageFormat *string `json:"errorMessageFormat,omitempty"`

	// ErrorMessageFormatSpecifierTypes The corresponding DQL format specifier types for each format specifier used in the error message format.
	ErrorMessageFormatSpecifierTypes *[]string `json:"errorMessageFormatSpecifierTypes,omitempty"`

	// ErrorType The error type, e.g. COMMAND_NAME_MISSING
	ErrorType *string `json:"errorType,omitempty"`

	// ExceptionType The exception type.
	ExceptionType *string `json:"exceptionType,omitempty"`

	// MissingPermissions List of missing IAM permissions necessary to successfully execute the request.
	MissingPermissions *[]string `json:"missingPermissions,omitempty"`

	// MissingScopes List of missing IAM scopes necessary to successfully execute the request.
	MissingScopes *[]string `json:"missingScopes,omitempty"`
	QueryId       *string   `json:"queryId,omitempty"`

	// QueryString Submitted query string.
	QueryString *string `json:"queryString,omitempty"`

	// SyntaxErrorPosition The position of a token in a query string used for errors and notification to map the message to a specific part of the query.
	SyntaxErrorPosition *TokenPosition `json:"syntaxErrorPosition,omitempty"`
}

// ExecuteRequest defines model for ExecuteRequest.
type ExecuteRequest struct {
	// DefaultSamplingRatio Default sampling ratio. By default no sampling is applied. No upper limit but will be normalized to a power of 10 less than or equal to 100000.
	DefaultSamplingRatio *float64 `json:"defaultSamplingRatio,omitempty"`

	// DefaultScanLimitGbytes Default scan limit. Can be overridden in DQL. Default value is configured on application level (see documentation of FETCH command). No upper limit. Use -1 for no limit.
	DefaultScanLimitGbytes *int32 `json:"defaultScanLimitGbytes,omitempty"`

	// DefaultTimeframeEnd The query timeframe 'end' timestamp in ISO-8601 or RFC3339 format. If the timeframe 'start' parameter is missing, the whole timeframe is ignored. *Note that if a timeframe is specified within the query string (query) then it has precedence over this query request parameter.*
	DefaultTimeframeEnd *string `json:"defaultTimeframeEnd,omitempty"`

	// DefaultTimeframeStart The query timeframe 'start' timestamp in ISO-8601 or RFC3339 format. If the timeframe 'end' parameter is missing, the whole timeframe is ignored. *Note that if a timeframe is specified within the query string (query) then it has precedence over this query request parameter.*
	DefaultTimeframeStart *string `json:"defaultTimeframeStart,omitempty"`

	// EnablePreview Request preview results. If a preview is available within the requestTimeoutMilliseconds, then it will be returned as part of the response.
	EnablePreview *bool `json:"enablePreview,omitempty"`

	// EnforceQueryConsumptionLimit Boolean to indicate if the query consumption limit should be enforced
	EnforceQueryConsumptionLimit *bool `json:"enforceQueryConsumptionLimit,omitempty"`

	// FetchTimeoutSeconds The time limit for fetching data. Soft limit as further data processing can happen. No upper limit in API but application level default and maximum fetch timeout also applies.
	FetchTimeoutSeconds *int32 `json:"fetchTimeoutSeconds,omitempty"`

	// FilterSegments Represents a collection of filter segments.
	FilterSegments *FilterSegments `json:"filterSegments,omitempty"`

	// IncludeContributions Indicates whether bucket contribution information should be included in the query response metadata. When set to true, the response will contain details about how each bucket contributed to the query result.
	IncludeContributions *bool `json:"includeContributions,omitempty"`

	// IncludeTypes Parameter to exclude the type information from the query result. In case not specified, the type information will be included.
	IncludeTypes *bool `json:"includeTypes,omitempty"`

	// Locale The query locale. If none specified, then a language/country neutral locale is chosen. The input values take the ISO-639 Language code with an optional ISO-3166 country code appended to it with an underscore. For instance, both values are valid 'en' or 'en_US'.
	Locale *string `json:"locale,omitempty"`

	// MaxResultBytes The maximum number of serialized result bytes. Applies to records only and is a soft limit, i.e. the last record that exceeds the limit will be included in the response completely. No upper limit, no default value.
	MaxResultBytes *int64 `json:"maxResultBytes,omitempty"`

	// MaxResultRecords The maximum number of returned query result records. No upper limit.
	MaxResultRecords *int64 `json:"maxResultRecords,omitempty"`

	// Query The full query string.
	Query string `json:"query"`

	// QueryOptions Query options enhance query functionality for Dynatrace internal services.
	QueryOptions *QueryOptions `json:"queryOptions,omitempty"`

	// RequestTimeoutMilliseconds The maximum time the response will be delayed to wait for a result. (This excludes the sending time and time spent in any services between the query-frontend and the client.) If the query finishes within the specified timeout, the query result is returned. Otherwise, the requestToken is returned, allowing polling for the result.
	RequestTimeoutMilliseconds *int64 `json:"requestTimeoutMilliseconds,omitempty"`

	// Timezone The query timezone. If none is specified, UTC is used as fallback. The list of valid input values matches that of the IANA Time Zone Database (TZDB). It accepts values in their canonical names like 'Europe/Paris', the abbreviated version like CET or the UTC offset format like '+01:00'
	Timezone *string `json:"timezone,omitempty"`
}

// FieldType The possible type of a field in DQL.
type FieldType struct {
	Type  FieldTypeType       `json:"type"`
	Types *[]RangedFieldTypes `json:"types,omitempty"`
}

// FieldTypeType defines model for FieldType.Type.
type FieldTypeType string

// FilterSegment A filter segment is identified by an ID. Each segment includes a list of variable definitions.
type FilterSegment struct {
	Id        string                             `json:"id"`
	Variables *[]FilterSegmentVariableDefinition `json:"variables,omitempty"`
}

// FilterSegmentVariableDefinition Defines a variable with a name and a list of values.
type FilterSegmentVariableDefinition struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// FilterSegments Represents a collection of filter segments.
type FilterSegments = []FilterSegment

// GeoPoint DQL data type representing a geolocation point.
type GeoPoint struct {
	// Latitude The coordinate that specifies the north-south position of a point on the surface of the earth.
	Latitude float32 `json:"latitude"`

	// Longitude The coordinate that specifies the  east-west position of a point on the surface of the earth.
	Longitude float32 `json:"longitude"`
}

// GrailMetadata Collects various bits of metadata information.
type GrailMetadata struct {
	// AnalysisTimeframe DQL data type timeframe.
	AnalysisTimeframe *Timeframe `json:"analysisTimeframe,omitempty"`

	// CanonicalQuery The canonical form of the query. It has normalized spaces and canonical constructs.
	CanonicalQuery *string        `json:"canonicalQuery,omitempty"`
	Contributions  *Contributions `json:"contributions,omitempty"`

	// DqlVersion The version of DQL that was used to process the query request.
	DqlVersion *string `json:"dqlVersion,omitempty"`

	// ExecutionTimeMilliseconds The time it took to execute the query.
	ExecutionTimeMilliseconds *int64 `json:"executionTimeMilliseconds,omitempty"`

	// Locale Effective locale for the query.
	Locale *string `json:"locale,omitempty"`

	// Notifications Collected messages during the execution of the query.
	Notifications *[]MetadataNotification `json:"notifications,omitempty"`

	// Query The submitted query.
	Query *string `json:"query,omitempty"`

	// QueryId The id of the query
	QueryId *string `json:"queryId,omitempty"`

	// Sampled True if sampling was used for at least one segment.
	Sampled *bool `json:"sampled,omitempty"`

	// ScannedBytes Number of scanned bytes during the query execution.
	ScannedBytes      *int64 `json:"scannedBytes,omitempty"`
	ScannedDataPoints *int64 `json:"scannedDataPoints,omitempty"`

	// ScannedRecords Number of scanned records during the query execution.
	ScannedRecords *int64 `json:"scannedRecords,omitempty"`

	// Timezone Effective timezone for the query.
	Timezone *string `json:"timezone,omitempty"`
}

// Metadata Collects various bits of metadata information.
type Metadata struct {
	// Grail Collects various bits of metadata information.
	Grail   *GrailMetadata    `json:"grail,omitempty"`
	Metrics *[]MetricMetadata `json:"metrics,omitempty"`
}

// MetadataNotification The message that provides additional information about the execution of the query.
type MetadataNotification struct {
	// Arguments The arguments for the message format.
	Arguments *[]string `json:"arguments,omitempty"`

	// Message The complete message of the notification.
	Message *string `json:"message,omitempty"`

	// MessageFormat The message format of the notification, string.format based
	MessageFormat *string `json:"messageFormat,omitempty"`

	// MessageFormatSpecifierTypes The corresponding DQL format specifier types for each format specifier used in the error message format.
	MessageFormatSpecifierTypes *[]string `json:"messageFormatSpecifierTypes,omitempty"`

	// NotificationType The notification type, e.g. LIMIT_ADDED.
	NotificationType *string `json:"notificationType,omitempty"`

	// Severity The severity of the notification, currently: INFO, WARN, ERROR.
	Severity *string `json:"severity,omitempty"`

	// SyntaxPosition The position of a token in a query string used for errors and notification to map the message to a specific part of the query.
	SyntaxPosition *TokenPosition `json:"syntaxPosition,omitempty"`
}

// MetricMetadata An object that defines additional metric metadata.
type MetricMetadata struct {
	// Description The description of the metadata.
	Description *string `json:"description,omitempty"`

	// DisplayName The display name of the metadata.
	DisplayName *string `json:"displayName,omitempty"`

	// FieldName The name of the associated field used in the query.
	FieldName *string `json:"fieldName,omitempty"`

	// MetricKey The metric key.
	MetricKey *string `json:"metric.key,omitempty"`

	// Rate The specified rate normalization parameter.
	Rate *string `json:"rate,omitempty"`

	// Rollup Metadata about the rollup type.
	Rollup *string `json:"rollup,omitempty"`

	// Scalar Indicates whether the scalar parameter was set to true in the timeseries aggregation function.
	Scalar *bool `json:"scalar,omitempty"`

	// Shifted Indicates if the shifted parameter was used.
	Shifted *bool `json:"shifted,omitempty"`

	// Unit The unit used.
	Unit *string `json:"unit,omitempty"`
}

// ParseRequest defines model for ParseRequest.
type ParseRequest struct {
	// Locale The query locale. If none specified, then a language/country neutral locale is chosen. The input values take the ISO-639 Language code with an optional ISO-3166 country code appended to it with an underscore. For instance, both values are valid 'en' or 'en_US'.
	Locale *string `json:"locale,omitempty"`

	// Query The full query string.
	Query string `json:"query"`

	// QueryOptions Query options enhance query functionality for Dynatrace internal services.
	QueryOptions *QueryOptions `json:"queryOptions,omitempty"`

	// Timezone The query timezone. If none is specified, UTC is used as fallback. The list of valid input values matches that of the IANA Time Zone Database (TZDB). It accepts values in their canonical names like 'Europe/Paris', the abbreviated version like CET or the UTC offset format like '+01:00'
	Timezone *string `json:"timezone,omitempty"`
}

// PositionInfo The exact position in the query string.
type PositionInfo struct {
	// Column Query position column zero based index.
	Column int32 `json:"column"`

	// Index Query position index.
	Index int32 `json:"index"`

	// Line Query position line zero based index.
	Line int32 `json:"line"`
}

// QueryOptions Query options enhance query functionality for Dynatrace internal services.
type QueryOptions map[string]string

// QueryPollResponse The response of GET query:execute call.
type QueryPollResponse struct {
	// Progress The progress of the query from 0 to 100.
	Progress *int32 `json:"progress,omitempty"`

	// Result The result of the DQL query.
	Result *QueryResult `json:"result,omitempty"`

	// State Possible state of the query.
	State QueryState `json:"state"`

	// TtlSeconds Time to live in seconds.
	TtlSeconds *int64 `json:"ttlSeconds,omitempty"`
}

// QueryResult The result of the DQL query.
type QueryResult struct {
	// Metadata Collects various bits of metadata information.
	Metadata Metadata `json:"metadata"`

	// Records List of records containing the result fields data.
	Records []*ResultRecord `json:"records"`

	// Types The data types for the result records.
	Types []RangedFieldTypes `json:"types"`
}

// QueryStartResponse The response when starting a query.
type QueryStartResponse struct {
	// Progress The progress of the query from 0 to 100.
	Progress *int32 `json:"progress,omitempty"`

	// RequestToken The token returned by the POST query:execute call.
	RequestToken *string `json:"requestToken,omitempty"`

	// Result The result of the DQL query.
	Result *QueryResult `json:"result,omitempty"`

	// State Possible state of the query.
	State QueryState `json:"state"`

	// TtlSeconds Time to live in seconds.
	TtlSeconds *int64 `json:"ttlSeconds,omitempty"`
}

// QueryState Possible state of the query.
type QueryState string

// RangedFieldTypes The field type in range.
type RangedFieldTypes struct {
	// IndexRange The range of elements at use this type in arrays (null for records).
	IndexRange *[]int32 `json:"indexRange,omitempty"`

	// Mappings The mapping between the field name and data type.
	Mappings map[string]FieldType `json:"mappings"`
}

// ResultRecord Single record that contains the result fields.
type ResultRecord map[string]*json.RawMessage

// ResultRecordValue Single result field of a record.
type ResultRecordValue struct {
	/**
	 * can be:
	 *   bool
	 *   number
	 *   string
	 *   Timeframe
	 *   GeoPoint
	 *   ResultRecord
	 *   array
	 */
	union json.RawMessage
}

// Timeframe DQL data type timeframe.
type Timeframe struct {
	// End The end time of the timeframe.
	End *time.Time `json:"end,omitempty"`

	// Start The start time of the timeframe.
	Start *time.Time `json:"start,omitempty"`
}

// TokenPosition The position of a token in a query string used for errors and notification to map the message to a specific part of the query.
type TokenPosition struct {
	// End The exact position in the query string.
	End PositionInfo `json:"end"`

	// Start The exact position in the query string.
	Start PositionInfo `json:"start"`
}

// VerifyRequest defines model for VerifyRequest.
type VerifyRequest struct {
	GenerateCanonicalQuery *bool `json:"generateCanonicalQuery,omitempty"`

	// Locale The query locale. If none specified, then a language/country neutral locale is chosen. The input values take the ISO-639 Language code with an optional ISO-3166 country code appended to it with an underscore. For instance, both values are valid 'en' or 'en_US'.
	Locale *string `json:"locale,omitempty"`

	// Query The full query string.
	Query string `json:"query"`

	// QueryOptions Query options enhance query functionality for Dynatrace internal services.
	QueryOptions *QueryOptions `json:"queryOptions,omitempty"`

	// Timezone The query timezone. If none is specified, UTC is used as fallback. The list of valid input values matches that of the IANA Time Zone Database (TZDB). It accepts values in their canonical names like 'Europe/Paris', the abbreviated version like CET or the UTC offset format like '+01:00'
	Timezone *string `json:"timezone,omitempty"`
}

// VerifyResponse Verify response.
type VerifyResponse struct {
	CanonicalQuery *string `json:"canonicalQuery,omitempty"`

	// Notifications The notifications related to the supplied DQL query string.
	Notifications *[]MetadataNotification `json:"notifications,omitempty"`

	// Valid True if the supplied DQL query string is valid.
	Valid bool `json:"valid"`
}
