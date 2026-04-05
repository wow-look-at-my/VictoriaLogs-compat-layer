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
	// 1700000000 seconds = 2023-11-14T22:13:20Z
	got, err := TranslateTimestamp("1700000000000000000")
	require.Nil(t, err)

	want := "2023-11-14T22:13:20Z"
	assert.Equal(t, want, got)

	// Invalid input.
	_, err = TranslateTimestamp("not-a-number")
	assert.NotNil(t, err)
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

func TestBuildHitsRequest_EmptyQuery(t *testing.T) {
	backend, _ := url.Parse("http://localhost:9428")
	params := url.Values{}

	req, err := BuildHitsRequest(backend, params)
	require.Nil(t, err)

	q := req.URL.Query()
	assert.Equal(t, "*", q.Get("query"))
}
