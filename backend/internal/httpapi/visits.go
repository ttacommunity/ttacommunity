// The site's visit count, from Cloudflare Web Analytics.
package httpapi

import (
	"context"

	"ttacommunity/backend/internal/webanalytics"
)

// cacheHour is the visit count's lifetime: the API counts again every hour.
const cacheHour = "public, max-age=3600"

// Visits is the site's latest visit count, false while there is none to show.
type Visits interface {
	Latest() (webanalytics.Count, bool)
}

// GetSiteVisits returns the latest visit count: a 404 when this API does not count visits, and a 503 when it has
// no count from the last day.
func (s *server) GetSiteVisits(context.Context, GetSiteVisitsRequestObject) (GetSiteVisitsResponseObject, error) {
	if s.visits == nil {
		return GetSiteVisits404JSONResponse{Body: Error{Error: "this API does not count visits"}, Headers: GetSiteVisits404ResponseHeaders{CacheControl: cacheNone}}, nil
	}
	count, ok := s.visits.Latest()
	if !ok {
		return GetSiteVisits503JSONResponse{Body: Error{Error: "the visits have not been counted in the last day"}, Headers: GetSiteVisits503ResponseHeaders{CacheControl: cacheNone}}, nil
	}
	return GetSiteVisits200JSONResponse{
		Body:    SiteVisits{Visits: count.Visits, Days: count.Days, AsOf: count.AsOf.UTC()},
		Headers: GetSiteVisits200ResponseHeaders{CacheControl: cacheHour},
	}, nil
}
