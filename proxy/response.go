package proxy

import (
	"encoding/json"
	"fmt"
	"time"
)

// fieldNamesResponse is the VictoriaLogs response format shared by
// /select/logsql/field_names, /select/logsql/stream_field_names,
// and /select/logsql/stream_field_values.
type fieldNamesResponse struct {
	Values []fieldNameEntry `json:"values"`
}

type fieldNameEntry struct {
	Value string `json:"value"`
	Hits  uint64 `json:"hits"`
}

// hitsResponse is the VictoriaLogs /select/logsql/hits response format.
type hitsResponse struct {
	Hits []hitEntry `json:"hits"`
}

type hitEntry struct {
	Fields     map[string]string `json:"fields"`
	Timestamps []string          `json:"timestamps"`
	Values     []uint64          `json:"values"`
	Total      uint64            `json:"total"`
}

// lokiVolumeResponse is the Loki /loki/api/v1/index/volume response format
// (Prometheus instant vector).
type lokiVolumeResponse struct {
	Status string         `json:"status"`
	Data   lokiVolumeData `json:"data"`
}

type lokiVolumeData struct {
	ResultType string             `json:"resultType"`
	Result     []lokiVolumeResult `json:"result"`
}

type lokiVolumeResult struct {
	Metric map[string]string `json:"metric"`
	Value  [2]interface{}    `json:"value"`
}

// lokiDetectedLabelsResponse is the Loki /loki/api/v1/detected_labels response format.
type lokiDetectedLabelsResponse struct {
	Status string              `json:"status"`
	Data   []lokiDetectedLabel `json:"data"`
}

type lokiDetectedLabel struct {
	Label       string `json:"label"`
	Type        string `json:"type"`
	Cardinality uint64 `json:"cardinality"`
}

// lokiDetectedFieldsResponse is the Loki /loki/api/v1/detected_fields response format.
type lokiDetectedFieldsResponse struct {
	Fields []lokiDetectedField `json:"fields"`
}

type lokiDetectedField struct {
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Cardinality uint64   `json:"cardinality"`
	Parsers     []string `json:"parsers,omitempty"`
}

// lokiLabelsResponse is the Loki response for /labels and /label/{name}/values.
type lokiLabelsResponse struct {
	Status string   `json:"status"`
	Data   []string `json:"data"`
}

// lokiVolumeRangeResponse is the Loki /loki/api/v1/index/volume_range response
// format (Prometheus matrix).
type lokiVolumeRangeResponse struct {
	Status string              `json:"status"`
	Data   lokiVolumeRangeData `json:"data"`
}

type lokiVolumeRangeData struct {
	ResultType string                  `json:"resultType"`
	Result     []lokiVolumeRangeResult `json:"result"`
}

type lokiVolumeRangeResult struct {
	Metric map[string]string `json:"metric"`
	Values [][2]interface{}  `json:"values"`
}

// ConvertFieldNamesToDetectedLabels converts a VictoriaLogs field_names response
// into a Loki detected_labels response.
func ConvertFieldNamesToDetectedLabels(body []byte) ([]byte, error) {
	var fn fieldNamesResponse
	if err := json.Unmarshal(body, &fn); err != nil {
		return nil, fmt.Errorf("unmarshal field_names: %w", err)
	}

	labels := make([]lokiDetectedLabel, 0, len(fn.Values))
	for _, e := range fn.Values {
		labels = append(labels, lokiDetectedLabel{
			Label:       e.Value,
			Type:        "S",
			Cardinality: e.Hits,
		})
	}

	return json.Marshal(lokiDetectedLabelsResponse{Status: "success", Data: labels})
}

// ConvertFieldNamesToDetectedFields converts a VictoriaLogs field_names response
// into a Loki detected_fields response.
func ConvertFieldNamesToDetectedFields(body []byte) ([]byte, error) {
	var fn fieldNamesResponse
	if err := json.Unmarshal(body, &fn); err != nil {
		return nil, fmt.Errorf("unmarshal field_names: %w", err)
	}

	fields := make([]lokiDetectedField, 0, len(fn.Values))
	for _, e := range fn.Values {
		fields = append(fields, lokiDetectedField{
			Label:       e.Value,
			Type:        "string",
			Cardinality: e.Hits,
		})
	}

	return json.Marshal(lokiDetectedFieldsResponse{Fields: fields})
}

// ConvertFieldNamesToLabels converts a VictoriaLogs field_names/stream_field_names
// response into a Loki labels response (flat string array).
func ConvertFieldNamesToLabels(body []byte) ([]byte, error) {
	var fn fieldNamesResponse
	if err := json.Unmarshal(body, &fn); err != nil {
		return nil, fmt.Errorf("unmarshal field_names: %w", err)
	}

	data := make([]string, 0, len(fn.Values))
	for _, e := range fn.Values {
		data = append(data, e.Value)
	}

	return json.Marshal(lokiLabelsResponse{Status: "success", Data: data})
}

// ConvertHitsToVolumeRange converts a VictoriaLogs hits response into a Loki
// volume_range response (Prometheus matrix format).
func ConvertHitsToVolumeRange(hitsBody []byte) ([]byte, error) {
	var hits hitsResponse
	if err := json.Unmarshal(hitsBody, &hits); err != nil {
		return nil, fmt.Errorf("unmarshal hits: %w", err)
	}

	results := make([]lokiVolumeRangeResult, 0, len(hits.Hits))
	for _, h := range hits.Hits {
		values := make([][2]interface{}, 0, len(h.Timestamps))
		for i, tsStr := range h.Timestamps {
			t, err := parseTimestamp(tsStr)
			if err != nil {
				return nil, fmt.Errorf("parse timestamp %q: %w", tsStr, err)
			}
			var count uint64
			if i < len(h.Values) {
				count = h.Values[i]
			}
			values = append(values, [2]interface{}{float64(t.Unix()), fmt.Sprintf("%d", count)})
		}
		results = append(results, lokiVolumeRangeResult{
			Metric: h.Fields,
			Values: values,
		})
	}

	resp := lokiVolumeRangeResponse{
		Status: "success",
		Data: lokiVolumeRangeData{
			ResultType: "matrix",
			Result:     results,
		},
	}

	return json.Marshal(resp)
}

// ConvertHitsToVolume converts a VictoriaLogs hits response body into a Loki
// volume response (Prometheus instant vector format).
func ConvertHitsToVolume(hitsBody []byte, queryTime time.Time) ([]byte, error) {
	var hits hitsResponse
	if err := json.Unmarshal(hitsBody, &hits); err != nil {
		return nil, fmt.Errorf("unmarshal hits: %w", err)
	}

	results := make([]lokiVolumeResult, 0, len(hits.Hits))
	ts := float64(queryTime.Unix())

	for _, h := range hits.Hits {
		results = append(results, lokiVolumeResult{
			Metric: h.Fields,
			Value:  [2]interface{}{ts, fmt.Sprintf("%d", h.Total)},
		})
	}

	resp := lokiVolumeResponse{
		Status: "success",
		Data: lokiVolumeData{
			ResultType: "vector",
			Result:     results,
		},
	}

	return json.Marshal(resp)
}
