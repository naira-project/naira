package httpapi

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustEncodePageToken(t *testing.T, offset int, scope string) string {
	t.Helper()

	token, err := encodePageToken(offset, scope, nil)
	require.NoError(t, err)

	return token
}

func TestParseFilterSyntax(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantFilter *equalityFilter
		wantErr    error
	}{
		{
			name:       "empty filter means no filtering",
			input:      "   ",
			wantFilter: nil,
		},
		{
			name:  "supports quoted equality value and unescapes quotes",
			input: `field = "foo\"bar"`,
			wantFilter: &equalityFilter{
				field: "field",
				value: `foo"bar`,
			},
		},
		{
			name:    "rejects values that are not quoted strings",
			input:   "field = bare",
			wantErr: errInvalidFilter,
		},
		{
			name:    "rejects expressions without equality syntax",
			input:   `field`,
			wantErr: errInvalidFilter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, err := parseFilterSyntax(tt.input)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, filter)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantFilter, filter)
		})
	}
}

func TestListOptionsFromRequest(t *testing.T) {
	spec := listOptionsSpec{
		scope:         "nodes",
		allowedFields: map[string]bool{"provider": true},
	}

	tests := []struct {
		name    string
		path    string
		want    listOptions
		wantErr error
	}{
		{
			name: "defaults page size when omitted",
			path: "/v1/nodes",
			want: listOptions{
				pageSize: defaultPageSize,
				offset:   0,
			},
		},
		{
			name: "defaults page size when zero is explicitly requested",
			path: "/v1/nodes?pageSize=0",
			want: listOptions{
				pageSize: defaultPageSize,
				offset:   0,
			},
		},
		{
			name: "keeps supported filter and clamps oversized pages",
			path: "/v1/nodes?pageSize=1500&filter=provider=%22mlflow%22",
			want: listOptions{
				pageSize: maxPageSize,
				offset:   0,
				filter: &equalityFilter{
					field: "provider",
					value: "mlflow",
				},
			},
		},
		{
			name:    "rejects filters outside the allowed field list",
			path:    "/v1/nodes?filter=name=%22gpt-4%22",
			wantErr: errInvalidFilter,
		},
		{
			name:    "rejects negative page sizes",
			path:    "/v1/nodes?pageSize=-1",
			wantErr: errInvalidPageSize,
		},
		{
			name:    "rejects page token created for another scope",
			path:    "/v1/nodes?pageToken=" + mustEncodePageToken(t, 42, "relations"),
			wantErr: errInvalidPageToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			got, err := listOptionsFromRequest(req, spec)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPaginate(t *testing.T) {
	items := []string{"alpha", "beta", "gamma"}

	tests := []struct {
		name          string
		pageSize      int
		offset        int
		scope         string
		wantPage      []string
		wantNextToken string
		wantTotal     int
		wantErr       error
	}{
		{
			name:          "returns a page and token when more results remain",
			pageSize:      2,
			offset:        0,
			scope:         "nodes",
			wantPage:      []string{"alpha", "beta"},
			wantNextToken: mustEncodePageToken(t, 2, "nodes"),
			wantTotal:     3,
		},
		{
			name:      "last page has no next token",
			pageSize:  2,
			offset:    2,
			scope:     "nodes",
			wantPage:  []string{"gamma"},
			wantTotal: 3,
		},
		{
			name:     "rejects offsets beyond the collection size",
			pageSize: 2,
			offset:   4,
			scope:    "nodes",
			wantErr:  errInvalidPageToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, nextToken, total, err := paginate(items, tt.pageSize, tt.offset, tt.scope, nil)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, page)
				assert.Empty(t, nextToken)
				assert.Zero(t, total)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantPage, page)
			assert.Equal(t, tt.wantNextToken, nextToken)
			assert.Equal(t, tt.wantTotal, total)
		})
	}
}
