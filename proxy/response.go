package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
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

// vlQueryEntry is a single entry from VictoriaLogs NDJSON query response.
type vlQueryEntry struct {
	Msg    string `json:"_msg"`
	Time   string `json:"_time"`
	Stream string `json:"_stream"`
}

// lokiQueryResponse is the Loki query/query_range response format.
type lokiQueryResponse struct {
	Status string        `json:"status"`
	Data   lokiQueryData `json:"data"`
}

type lokiQueryData struct {
	ResultType string             `json:"resultType"`
	Result     []lokiStreamResult `json:"result"`
}

type lokiStreamResult struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

// lokiSeriesResponse is the Loki /loki/api/v1/series response format.
type lokiSeriesResponse struct {
	Status string              `json:"status"`
	Data   []map[string]string `json:"data"`
}

// lokiDetectedFieldValuesResponse is the Loki detected_field/{name}/values response format.
type lokiDetectedFieldValuesResponse struct {
	Values []string `json:"values"`
}

// vlStatsResponse is the VictoriaLogs /select/logsql/stats response format.
type vlStatsResponse struct {
	Streams uint64 `json:"streams"`
	Rows    uint64 `json:"rows"`
	Bytes   uint64 `json:"bytes"`
}

// lokiIndexStatsResponse is the Loki /loki/api/v1/index/stats response format.
type lokiIndexStatsResponse struct {
	Streams uint64 `json:"streams"`
	Chunks  uint64 `json:"chunks"`
	Entries uint64 `json:"entries"`
	Bytes   uint64 `json:"bytes"`
}

// parseStreamSelector parses a VictoriaLogs stream selector like
// {job="nginx",level="info"} into a map.
func parseStreamSelector(selector string) map[string]string {
	result := make(map[string]string)
	s := strings.TrimSpace(selector)
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return result
	}
	s = s[1 : len(s)-1]
	for len(s) > 0 {
		s = strings.TrimSpace(s)
		if len(s) == 0 {
			break
		}
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			break
		}
		key := strings.TrimSpace(s[:eq])
		s = strings.TrimSpace(s[eq+1:])
		if len(s) == 0 || s[0] != '"' {
			break
		}
		// Scan quoted value with backslash escape support.
		var val strings.Builder
		i := 1
		for i < len(s) {
			c := s[i]
			if c == '\\' && i+1 < len(s) {
				val.WriteByte(s[i+1])
				i += 2
				continue
			}
			if c == '"' {
				i++
				break
			}
			val.WriteByte(c)
			i++
		}
		result[key] = val.String()
		s = s[i:]
		s = strings.TrimSpace(s)
		if strings.HasPrefix(s, ",") {
			s = s[1:]
		}
	}
	return result
}

// ConvertQueryToStreams converts a VictoriaLogs NDJSON query response to a
// Loki streams response.
func ConvertQueryToStreams(body []byte) ([]byte, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return json.Marshal(lokiQueryResponse{
			Status: "success",
			Data:   lokiQueryData{ResultType: "streams", Result: []lokiStreamResult{}},
		})
	}

	// groups maps stream selector string → index in results slice, preserving
	// insertion order via results.
	groups := map[string]int{}
	results := []lokiStreamResult{}

	for _, line := range bytes.Split(body, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var entry vlQueryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("unmarshal query entry: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, entry.Time)
		if err != nil {
			// Fall back to RFC3339 without sub-seconds.
			t, err = time.Parse(time.RFC3339, entry.Time)
			if err != nil {
				return nil, fmt.Errorf("parse _time %q: %w", entry.Time, err)
			}
		}
		nsStr := fmt.Sprintf("%d", t.UnixNano())

		idx, ok := groups[entry.Stream]
		if !ok {
			idx = len(results)
			groups[entry.Stream] = idx
			results = append(results, lokiStreamResult{
				Stream: parseStreamSelector(entry.Stream),
				Values: [][2]string{},
			})
		}
		results[idx].Values = append(results[idx].Values, [2]string{nsStr, entry.Msg})
	}

	return json.Marshal(lokiQueryResponse{
		Status: "success",
		Data:   lokiQueryData{ResultType: "streams", Result: results},
	})
}

// ConvertStreamsToSeries converts a VictoriaLogs /select/logsql/streams
// response to a Loki series response.
func ConvertStreamsToSeries(body []byte) ([]byte, error) {
	var fn fieldNamesResponse
	if err := json.Unmarshal(body, &fn); err != nil {
		return nil, fmt.Errorf("unmarshal streams: %w", err)
	}

	data := make([]map[string]string, 0, len(fn.Values))
	for _, e := range fn.Values {
		data = append(data, parseStreamSelector(e.Value))
	}

	return json.Marshal(lokiSeriesResponse{Status: "success", Data: data})
}

// ConvertFieldValuesToDetectedFieldValues converts a VictoriaLogs
// /select/logsql/field_values response to a Loki detected field values response.
func ConvertFieldValuesToDetectedFieldValues(body []byte) ([]byte, error) {
	var fn fieldNamesResponse
	if err := json.Unmarshal(body, &fn); err != nil {
		return nil, fmt.Errorf("unmarshal field_values: %w", err)
	}

	values := make([]string, 0, len(fn.Values))
	for _, e := range fn.Values {
		values = append(values, e.Value)
	}

	return json.Marshal(lokiDetectedFieldValuesResponse{Values: values})
}

// ConvertStatsToIndexStats converts a VictoriaLogs /select/logsql/stats
// response to a Loki index stats response.
func ConvertStatsToIndexStats(body []byte) ([]byte, error) {
	var s vlStatsResponse
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("unmarshal stats: %w", err)
	}

	return json.Marshal(lokiIndexStatsResponse{
		Streams: s.Streams,
		Chunks:  0,
		Entries: s.Rows,
		Bytes:   s.Bytes,
	})
}
