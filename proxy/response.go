package proxy

import (
	"encoding/json"
	"fmt"
	"time"
)

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
