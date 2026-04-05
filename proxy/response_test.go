package proxy

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/wow-look-at-my/testify/assert"
	"github.com/wow-look-at-my/testify/require"
)

func TestConvertHitsToVolume_SingleHit(t *testing.T) {
	input := `{
		"hits": [
			{
				"fields": {"job": "nginx"},
				"timestamps": ["2023-11-14T22:00:00Z", "2023-11-14T23:00:00Z"],
				"values": [100, 250],
				"total": 350
			}
		]
	}`

	queryTime := time.Unix(1700000000, 0)
	result, err := ConvertHitsToVolume([]byte(input), queryTime)
	require.Nil(t, err)

	var resp lokiVolumeResponse
	require.NoError(t, json.Unmarshal(result, &resp))

	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "vector", resp.Data.ResultType)
	require.Equal(t, 1, len(resp.Data.Result))

	r := resp.Data.Result[0]
	assert.Equal(t, "nginx", r.Metric["job"])

	valStr, ok := r.Value[1].(string)
	require.True(t, ok)
	assert.Equal(t, "350", valStr)
}

func TestConvertHitsToVolume_MultipleHits(t *testing.T) {
	input := `{
		"hits": [
			{"fields": {"job": "nginx"}, "timestamps": [], "values": [], "total": 100},
			{"fields": {"job": "app"}, "timestamps": [], "values": [], "total": 200}
		]
	}`

	result, err := ConvertHitsToVolume([]byte(input), time.Now())
	require.Nil(t, err)

	var resp lokiVolumeResponse
	require.NoError(t, json.Unmarshal(result, &resp))
	require.Equal(t, 2, len(resp.Data.Result))
}

func TestConvertHitsToVolume_Empty(t *testing.T) {
	input := `{"hits": []}`

	result, err := ConvertHitsToVolume([]byte(input), time.Now())
	require.Nil(t, err)

	var resp lokiVolumeResponse
	require.NoError(t, json.Unmarshal(result, &resp))
	assert.Equal(t, 0, len(resp.Data.Result))
}

func TestConvertHitsToVolume_InvalidJSON(t *testing.T) {
	_, err := ConvertHitsToVolume([]byte("not json"), time.Now())
	assert.NotNil(t, err)
}

func TestConvertFieldNamesToDetectedLabels(t *testing.T) {
	input := `{"values":[{"value":"job","hits":100},{"value":"level","hits":50}]}`

	result, err := ConvertFieldNamesToDetectedLabels([]byte(input))
	require.Nil(t, err)

	var resp lokiDetectedLabelsResponse
	require.NoError(t, json.Unmarshal(result, &resp))
	assert.Equal(t, "success", resp.Status)
	require.Equal(t, 2, len(resp.Data))
	assert.Equal(t, "job", resp.Data[0].Label)
	assert.Equal(t, "S", resp.Data[0].Type)
	assert.Equal(t, uint64(100), resp.Data[0].Cardinality)
	assert.Equal(t, "level", resp.Data[1].Label)
}

func TestConvertFieldNamesToDetectedLabels_Empty(t *testing.T) {
	result, err := ConvertFieldNamesToDetectedLabels([]byte(`{"values":[]}`))
	require.Nil(t, err)

	var resp lokiDetectedLabelsResponse
	require.NoError(t, json.Unmarshal(result, &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, 0, len(resp.Data))
}

func TestConvertFieldNamesToDetectedLabels_InvalidJSON(t *testing.T) {
	_, err := ConvertFieldNamesToDetectedLabels([]byte("not json"))
	assert.NotNil(t, err)
}

func TestConvertFieldNamesToDetectedFields(t *testing.T) {
	input := `{"values":[{"value":"method","hits":200},{"value":"status","hits":80}]}`

	result, err := ConvertFieldNamesToDetectedFields([]byte(input))
	require.Nil(t, err)

	var resp lokiDetectedFieldsResponse
	require.NoError(t, json.Unmarshal(result, &resp))
	require.Equal(t, 2, len(resp.Fields))
	assert.Equal(t, "method", resp.Fields[0].Label)
	assert.Equal(t, "string", resp.Fields[0].Type)
	assert.Equal(t, uint64(200), resp.Fields[0].Cardinality)
	assert.Nil(t, resp.Fields[0].Parsers)
}

func TestConvertFieldNamesToDetectedFields_Empty(t *testing.T) {
	result, err := ConvertFieldNamesToDetectedFields([]byte(`{"values":[]}`))
	require.Nil(t, err)

	var resp lokiDetectedFieldsResponse
	require.NoError(t, json.Unmarshal(result, &resp))
	assert.Equal(t, 0, len(resp.Fields))
}

func TestConvertFieldNamesToLabels(t *testing.T) {
	input := `{"values":[{"value":"job","hits":100},{"value":"env","hits":50},{"value":"host","hits":30}]}`

	result, err := ConvertFieldNamesToLabels([]byte(input))
	require.Nil(t, err)

	var resp lokiLabelsResponse
	require.NoError(t, json.Unmarshal(result, &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, []string{"job", "env", "host"}, resp.Data)
}

func TestConvertFieldNamesToLabels_Empty(t *testing.T) {
	result, err := ConvertFieldNamesToLabels([]byte(`{"values":[]}`))
	require.Nil(t, err)

	var resp lokiLabelsResponse
	require.NoError(t, json.Unmarshal(result, &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, 0, len(resp.Data))
}

func TestConvertFieldNamesToLabels_InvalidJSON(t *testing.T) {
	_, err := ConvertFieldNamesToLabels([]byte("not json"))
	assert.NotNil(t, err)
}

func TestConvertHitsToVolumeRange(t *testing.T) {
	input := `{
		"hits": [
			{
				"fields": {"job": "nginx"},
				"timestamps": ["2023-11-14T22:00:00Z", "2023-11-14T23:00:00Z"],
				"values": [100, 250],
				"total": 350
			}
		]
	}`

	result, err := ConvertHitsToVolumeRange([]byte(input))
	require.Nil(t, err)

	var resp lokiVolumeRangeResponse
	require.NoError(t, json.Unmarshal(result, &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "matrix", resp.Data.ResultType)
	require.Equal(t, 1, len(resp.Data.Result))

	r := resp.Data.Result[0]
	assert.Equal(t, "nginx", r.Metric["job"])
	require.Equal(t, 2, len(r.Values))
	assert.Equal(t, "100", r.Values[0][1].(string))
	assert.Equal(t, "250", r.Values[1][1].(string))
}

func TestConvertHitsToVolumeRange_Empty(t *testing.T) {
	result, err := ConvertHitsToVolumeRange([]byte(`{"hits":[]}`))
	require.Nil(t, err)

	var resp lokiVolumeRangeResponse
	require.NoError(t, json.Unmarshal(result, &resp))
	assert.Equal(t, 0, len(resp.Data.Result))
}

func TestConvertHitsToVolumeRange_InvalidJSON(t *testing.T) {
	_, err := ConvertHitsToVolumeRange([]byte("not json"))
	assert.NotNil(t, err)
}
