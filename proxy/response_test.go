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
