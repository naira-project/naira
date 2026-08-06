package httpapi

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

var (
	errInvalidPageToken  = errors.New("invalid page token")
	errPageTokenEncoding = errors.New("page token encoding failed")
	errInvalidFilter     = errors.New("invalid filter")
	errInvalidOrderBy    = errors.New("invalid order_by")
	errInvalidPageSize   = errors.New("invalid pageSize")
)

const defaultPageSize = 50
const maxPageSize = 1000

type listOptions struct {
	pageSize int
	offset   int
	filter   *equalityFilter
}

type listOptionsSpec struct {
	scope         string
	allowedFields map[string]bool
}

type equalityFilter struct {
	field string
	value string
}

type pageTokenPayload struct {
	Offset int
	Scope  string
}

// Filter syntax is intentionally narrow for now: a single equality expression
// in the form field="value".
//
// Unsupported forms include chained expressions (AND/OR), inequality
// operators, and unquoted values.

func (filter *equalityFilter) matchesResource(fields map[string]string, resourceName string) (bool, error) {
	if filter == nil {
		return true, nil
	}

	actualValue, ok := fields[filter.field]
	if !ok {
		return false, fmt.Errorf("%w: unsupported %s filter field %q", errInvalidFilter, resourceName, filter.field)
	}

	return actualValue == filter.value, nil
}

func listOptionsFromRequest(r *http.Request, spec listOptionsSpec) (listOptions, error) {
	return listOptionsFromParams(
		r.URL.Query().Get("pageSize"),
		r.URL.Query().Get("filter"),
		r.URL.Query().Get("pageToken"),
		spec,
	)
}

func listOptionsFromParams(rawPageSize, rawFilter, rawPageToken string, spec listOptionsSpec) (listOptions, error) {
	pageSize := int32(0)
	if rawPageSize = strings.TrimSpace(rawPageSize); rawPageSize != "" {
		parsed, err := strconv.ParseInt(rawPageSize, 10, 32)
		if err != nil {
			return listOptions{}, fmt.Errorf("%w: %w", errInvalidPageSize, err)
		}
		pageSize = int32(parsed)
		if pageSize < 0 {
			return listOptions{}, fmt.Errorf("%w: must be >= 0", errInvalidPageSize)
		}
	}

	resolvedPageSize := int(pageSize)
	if resolvedPageSize == 0 {
		resolvedPageSize = defaultPageSize
	}
	if resolvedPageSize > maxPageSize {
		resolvedPageSize = maxPageSize
	}

	filter, err := parseFilterSyntax(rawFilter)
	if err != nil {
		return listOptions{}, fmt.Errorf("parsing filter: %w", err)
	}

	if filter != nil && !spec.allowedFields[filter.field] {
		return listOptions{}, fmt.Errorf("%w: unsupported %s filter field %q", errInvalidFilter, spec.scope, filter.field)
	}

	offset, err := decodePageToken(rawPageToken, spec.scope)
	if err != nil {
		return listOptions{}, fmt.Errorf("decoding page token: %w", err)
	}

	return listOptions{
		pageSize: resolvedPageSize,
		offset:   offset,
		filter:   filter,
	}, nil
}

func sortResources[T any](items []T, nameFor func(T) string) {
	sort.Slice(items, func(i, j int) bool {
		return nameFor(items[i]) < nameFor(items[j])
	})
}

func parseFilterSyntax(filter string) (*equalityFilter, error) {
	trimmed := strings.TrimSpace(filter)
	if trimmed == "" {
		return nil, nil
	}

	field, value, ok := strings.Cut(trimmed, "=")
	if !ok {
		return nil, fmt.Errorf(`%w: expected field="value"`, errInvalidFilter)
	}
	field = strings.TrimSpace(field)
	value = strings.TrimSpace(value)
	if field == "" || len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return nil, fmt.Errorf(`%w: expected field="value"`, errInvalidFilter)
	}

	unquotedValue, err := strconv.Unquote(value)
	if err != nil {
		return nil, fmt.Errorf(`%w: expected field="value"`, errInvalidFilter)
	}

	return &equalityFilter{field: field, value: unquotedValue}, nil
}

func paginate[T any](items []T, pageSize int, offset int, scope string, logger *log.Logger) ([]T, string, int, error) {
	totalSize := len(items)
	if offset < 0 || offset > totalSize {
		return nil, "", 0, fmt.Errorf("offset %d out of bounds (total %d): %w", offset, totalSize, errInvalidPageToken)
	}

	end := min(offset+pageSize, totalSize)

	page := append([]T(nil), items[offset:end]...)
	nextPageToken := ""
	if end < totalSize {
		var err error
		nextPageToken, err = encodePageToken(end, scope, logger)
		if err != nil {
			return nil, "", 0, fmt.Errorf("encoding next page token: %w", err)
		}
	}

	return page, nextPageToken, totalSize, nil
}

func encodePageToken(offset int, scope string, logger *log.Logger) (string, error) {
	payload := pageTokenPayload{Offset: offset, Scope: scope}

	var buf bytes.Buffer
	// gob used to obscure the token. Requirement for this described in AIP-158 #Opacity
	if err := gob.NewEncoder(&buf).Encode(payload); err != nil {
		if logger != nil {
			logger.Printf("encoding page token payload for scope %q at offset %d: %v", scope, offset, err)
		}
		return "", errPageTokenEncoding
	}

	return base64.RawURLEncoding.EncodeToString(buf.Bytes()), nil
}

func decodePageToken(pageToken string, scope string) (int, error) {
	trimmed := strings.TrimSpace(pageToken)
	if trimmed == "" {
		return 0, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(trimmed)
	if err != nil {
		return 0, errInvalidPageToken
	}

	var payload pageTokenPayload
	if err := gob.NewDecoder(bytes.NewReader(decoded)).Decode(&payload); err != nil {
		return 0, errInvalidPageToken
	}
	if payload.Scope != scope {
		return 0, fmt.Errorf("validating page token scope: %w", errInvalidPageToken)
	}

	return payload.Offset, nil
}
