package proxy

import (
	"net/url"
	"testing"

	"github.com/wow-look-at-my/testify/assert"
	"github.com/wow-look-at-my/testify/require"
)

func TestTranslateQuery(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`{job="foo"}`, `_stream:{job="foo"}`},
		{`{job="foo", env=~".+"}`, `_stream:{job="foo", env=~".+"}`},
		{"", "*"},
		{"  ", "*"},
	}
	for _, tt := range tests {
		got := TranslateQuery(tt.input)
		assert.Equal(t, tt.want, got)
	}
}

func TestTranslateTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "nanosecond epoch",
			input: "1700000000000000000",
			want:  "2023-11-14T22:13:20Z",
		},
		{
			name:  "RFC3339",
			input: "2023-11-14T22:13:20Z",
			want:  "2023-11-14T22:13:20Z",
		},
		{
			name:  "RFC3339 with milliseconds",
			input: "2026-04-05T05:23:26.735Z",
			want:  "2026-04-05T05:23:26Z",
		},
		{
			name:    "invalid",
			input:   "not-a-number",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TranslateTimestamp(tt.input)
			if tt.wantErr {
				assert.NotNil(t, err)
				return
			}
			require.Nil(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuildHitsRequest(t *testing.T) {
	backend, _ := url.Parse("http://localhost:9428")
	params := url.Values{
		"query":        {`{job="test"}`},
		"start":        {"1700000000000000000"},
		"end":          {"1700003600000000000"},
		"limit":        {"50"},
		"targetLabels": {"job,env"},
	}

	req, err := BuildHitsRequest(backend, params)
	require.Nil(t, err)

	assert.Equal(t, "/select/logsql/hits", req.URL.Path)

	q := req.URL.Query()

	assert.Equal(t, `_stream:{job="test"}`, q.Get("query"))
	assert.Equal(t, "2023-11-14T22:13:20Z", q.Get("start"))
	assert.Equal(t, "2023-11-14T23:13:20Z", q.Get("end"))
	assert.Equal(t, "50", q.Get("fields_limit"))

	fields := q["field"]
	assert.Equal(t, []string{"job", "env"}, fields)

	assert.NotEqual(t, "", q.Get("step"))
}

func TestBuildHitsRequest_RFC3339Timestamps(t *testing.T) {
	backend, _ := url.Parse("http://localhost:9428")
	params := url.Values{
		"query": {`{job="test"}`},
		"start": {"2023-11-14T22:13:20Z"},
		"end":   {"2023-11-14T23:13:20Z"},
	}

	req, err := BuildHitsRequest(backend, params)
	require.Nil(t, err)

	q := req.URL.Query()
	assert.Equal(t, "2023-11-14T22:13:20Z", q.Get("start"))
	assert.Equal(t, "2023-11-14T23:13:20Z", q.Get("end"))
	assert.Equal(t, "1h0m0s", q.Get("step"))
}

func TestBuildHitsRequest_EmptyQuery(t *testing.T) {
	backend, _ := url.Parse("http://localhost:9428")
	params := url.Values{}

	req, err := BuildHitsRequest(backend, params)
	require.Nil(t, err)

	q := req.URL.Query()
	assert.Equal(t, "*", q.Get("query"))
}

func TestBuildFieldNamesRequest(t *testing.T) {
	backend, _ := url.Parse("http://localhost:9428")
	params := url.Values{
		"query": {`{job="test"}`},
		"start": {"1700000000000000000"},
		"end":   {"1700003600000000000"},
	}

	req, err := BuildFieldNamesRequest(backend, params)
	require.Nil(t, err)

	assert.Equal(t, "/select/logsql/field_names", req.URL.Path)
	q := req.URL.Query()
	assert.Equal(t, `_stream:{job="test"}`, q.Get("query"))
	assert.Equal(t, "2023-11-14T22:13:20Z", q.Get("start"))
	assert.Equal(t, "2023-11-14T23:13:20Z", q.Get("end"))
}

func TestBuildFieldNamesRequest_EmptyQuery(t *testing.T) {
	backend, _ := url.Parse("http://localhost:9428")
	params := url.Values{}

	req, err := BuildFieldNamesRequest(backend, params)
	require.Nil(t, err)

	q := req.URL.Query()
	assert.Equal(t, "*", q.Get("query"))
}

func TestBuildStreamFieldNamesRequest(t *testing.T) {
	backend, _ := url.Parse("http://localhost:9428")
	params := url.Values{
		"query": {`{job="test"}`},
		"start": {"1700000000000000000"},
		"end":   {"1700003600000000000"},
	}

	req, err := BuildStreamFieldNamesRequest(backend, params)
	require.Nil(t, err)

	assert.Equal(t, "/select/logsql/stream_field_names", req.URL.Path)
	q := req.URL.Query()
	assert.Equal(t, `_stream:{job="test"}`, q.Get("query"))
}

func TestBuildStreamFieldValuesRequest(t *testing.T) {
	backend, _ := url.Parse("http://localhost:9428")
	params := url.Values{
		"query": {`{job="test"}`},
		"start": {"1700000000000000000"},
		"end":   {"1700003600000000000"},
	}

	req, err := BuildStreamFieldValuesRequest(backend, params, "job")
	require.Nil(t, err)

	assert.Equal(t, "/select/logsql/stream_field_values", req.URL.Path)
	q := req.URL.Query()
	assert.Equal(t, `_stream:{job="test"}`, q.Get("query"))
	assert.Equal(t, "job", q.Get("field_name"))
}

func TestBuildHitsRangeRequest(t *testing.T) {
	backend, _ := url.Parse("http://localhost:9428")
	params := url.Values{
		"query":        {`{job="test"}`},
		"start":        {"1700000000000000000"},
		"end":          {"1700003600000000000"},
		"step":         {"5m"},
		"limit":        {"50"},
		"targetLabels": {"job,env"},
	}

	req, err := BuildHitsRangeRequest(backend, params)
	require.Nil(t, err)

	assert.Equal(t, "/select/logsql/hits", req.URL.Path)
	q := req.URL.Query()
	assert.Equal(t, `_stream:{job="test"}`, q.Get("query"))
	assert.Equal(t, "5m", q.Get("step"))
	assert.Equal(t, "50", q.Get("fields_limit"))
	assert.Equal(t, []string{"job", "env"}, q["field"])
}

func TestBuildHitsRangeRequest_DefaultStep(t *testing.T) {
	backend, _ := url.Parse("http://localhost:9428")
	params := url.Values{}

	req, err := BuildHitsRangeRequest(backend, params)
	require.Nil(t, err)

	q := req.URL.Query()
	assert.Equal(t, "1h", q.Get("step"))
}
