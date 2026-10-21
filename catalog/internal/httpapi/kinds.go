package httpapi

import (
	"fmt"
	"log"
	"net/http"

	"github.com/naira-project/naira/catalog/internal/catalog"
)

type ListKindsResponse struct {
	Kinds         []string `json:"kinds"`
	NextPageToken string   `json:"nextPageToken,omitempty"`
	TotalSize     int32    `json:"totalSize"`
}

var kindListOptionsSpec = listOptionsSpec{
	scope: "kinds",
}

// newListKindsHandler lists distinct catalog relation kinds.
// Supported query params:
// - pageSize
// - pageToken
func newListKindsHandler(service *catalog.Service, logger *log.Logger) http.HandlerFunc {
	return handleWithListOptions(kindListOptionsSpec, func(w http.ResponseWriter, r *http.Request, options listOptions) error {
		kinds := service.ListKinds(r.Context())

		page, nextPageToken, totalSize, err := paginate(kinds, options.pageSize, options.offset, "kinds", logger)
		if err != nil {
			return fmt.Errorf("paginating kinds: %w", err)
		}
		if page == nil {
			page = []string{}
		}

		writeJSON(w, http.StatusOK, ListKindsResponse{Kinds: page, NextPageToken: nextPageToken, TotalSize: int32FromCount(totalSize, logger)})
		return nil
	})
}
