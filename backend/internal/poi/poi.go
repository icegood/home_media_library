package poi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// CategoryID names one of the toggleable POI categories in the map UI.
type CategoryID string

const (
	CategoryFood        CategoryID = "food"
	CategoryFuelParking CategoryID = "fuel_parking"
	CategoryLodging     CategoryID = "lodging"
	CategoryAttraction  CategoryID = "attraction"
	CategoryHealth      CategoryID = "health"
	CategoryShops       CategoryID = "shops"
)

// ValidCategories is the fixed set of categories the client can toggle.
var ValidCategories = map[CategoryID]bool{
	CategoryFood: true, CategoryFuelParking: true, CategoryLodging: true,
	CategoryAttraction: true, CategoryHealth: true, CategoryShops: true,
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// BBox is a bounding box in WGS-84 degrees (west < east, south < north).
type BBox struct {
	West, South, East, North float64
}

// POI is a point of interest returned by any provider.
type POI struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Category       CategoryID `json:"category"`
	Lat            float64    `json:"lat"`
	Lon            float64    `json:"lon"`
	Website        string     `json:"website,omitempty"`
	WikipediaTitle string     `json:"wikipediaTitle,omitempty"`
	ImageURL       string     `json:"imageUrl,omitempty"`
}

// Fetch returns POIs inside bbox for the given provider and categories.
// providerKey carries the provider-specific config from server settings
// (endpoint for overpass, apiKey for geoapify, token for mapbox).
func Fetch(ctx context.Context, provider, providerKey string, bbox BBox, categories []CategoryID) ([]POI, error) {
	if len(categories) == 0 {
		return nil, nil
	}
	if cached, ok := cacheGet(provider, providerKey, bbox, categories); ok {
		return cached, nil
	}
	var (
		out []POI
		err error
	)
	switch provider {
	case "overpass":
		out, err = fetchOverpass(ctx, providerKey, bbox, categories)
	case "geoapify":
		out, err = fetchGeoapify(ctx, providerKey, bbox, categories)
	case "mapbox":
		out, err = fetchMapbox(ctx, providerKey, bbox, categories)
	default:
		return nil, fmt.Errorf("poi: unknown provider %q", provider)
	}
	if err != nil {
		return nil, err
	}
	cacheSet(provider, providerKey, bbox, categories, out)
	return out, nil
}

// ── Overpass ──────────────────────────────────────────────────────────────────

// overpassConditions returns the OSM tag conditions for a category. Each
// condition is emitted as its own node/way/relation union statement (Overpass
// tag brackets only accept one key condition each).
func overpassConditions(cat CategoryID) []string {
	switch cat {
	case CategoryFood:
		return []string{`"amenity"~"restaurant|cafe|fast_food|pub|bar"`}
	case CategoryFuelParking:
		return []string{`"amenity"~"fuel|charging_station|parking"`}
	case CategoryLodging:
		return []string{`"tourism"~"hotel|hostel|motel|camp_site"`}
	case CategoryAttraction:
		return []string{`"tourism"~"attraction|museum|viewpoint"`, `"leisure"="park"`, `"natural"="waterfall"`}
	case CategoryHealth:
		return []string{`"amenity"~"pharmacy|hospital|dentist|clinic|doctors|atm|bank"`}
	case CategoryShops:
		return []string{`"shop"~"supermarket|convenience|mall"`}
	default:
		return nil
	}
}

type overpassResponse struct {
	Elements []struct {
		ID    int64              `json:"id"`
		Type  string             `json:"type"`
		Lat   float64            `json:"lat"`
		Lon   float64            `json:"lon"`
		Tags  map[string]string  `json:"tags"`
		Center *struct{ Lat, Lon float64 } `json:"center"`
	} `json:"elements"`
}

// overpassEndpoints is the ordered list of public Overpass mirrors tried in
// sequence. Each gets a very short budget so the UI fails fast instead of
// hanging on "Loading POI…".
var overpassEndpoints = []string{
	"https://overpass-api.de/api/interpreter",
	"https://overpass.kumi.systems/api/interpreter",
}

// perOverpassTimeout caps a single mirror attempt to 3 seconds. If Overpass
// can't answer in 3s it's effectively down for our purposes.
const perOverpassTimeout = 3 * time.Second

func fetchOverpass(ctx context.Context, endpoint string, bbox BBox, categories []CategoryID) ([]POI, error) {
	query, buildErr := buildOverpassQuery(bbox, categories)
	if buildErr != nil {
		return nil, buildErr
	}
	// Explicit admin/env endpoint takes precedence and is tried alone.
	if url := firstNonEmpty(endpoint, os.Getenv("POI_OVERPASS_ENDPOINT")); url != "" {
		c, cancel := context.WithTimeout(ctx, perOverpassTimeout)
		defer cancel()
		return runOverpassQuery(c, url, query, categories)
	}
	// Walk the built-in list; stop at the first success or non-network error.
	for _, url := range overpassEndpoints {
		c, cancel := context.WithTimeout(ctx, perOverpassTimeout)
		pois, err := runOverpassQuery(c, url, query, categories)
		cancel()
		if err == nil {
			return pois, nil
		}
		// A structural 4xx (query rejected, auth problem) is final —
		// retrying a different mirror won't help.
		if strings.Contains(err.Error(), "overpass 4") {
			return nil, err
		}
	}
	return nil, fmt.Errorf("Overpass is unreachable from this server (tried %d mirrors in %v). Check network access or set a custom endpoint in Admin → Maps → POI providers", len(overpassEndpoints), perOverpassTimeout*time.Duration(len(overpassEndpoints)))
}

// buildOverpassQuery assembles the Overpass QL query that unions node +
// way + relation statements for each category × OSM tag-condition.
func buildOverpassQuery(bbox BBox, categories []CategoryID) (string, error) {
	if len(categories) == 0 {
		return "", fmt.Errorf("poi: no categories requested")
	}
	var parts []string
	for _, cat := range categories {
		for _, cond := range overpassConditions(cat) {
			if cond == "" {
				continue
			}
			b := fmt.Sprintf(`(node[%s](%f,%f,%f,%f);way[%s](%f,%f,%f,%f);relation[%s](%f,%f,%f,%f););`,
				cond, bbox.South, bbox.West, bbox.North, bbox.East,
				cond, bbox.South, bbox.West, bbox.North, bbox.East,
				cond, bbox.South, bbox.West, bbox.North, bbox.East)
			parts = append(parts, b)
		}
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("poi: no usable conditions for %v", categories)
	}
	return fmt.Sprintf(`[out:json][timeout:25][bbox:%f,%f,%f,%f];%s out center;`,
		bbox.South, bbox.West, bbox.North, bbox.East,
		strings.Join(parts, "")), nil
}

// runOverpassQuery POSTs the query to a single mirror and decodes its JSON.
// Network errors come back un-wrapped; HTTP-status errors come back wrapped
// with the upstream body so the caller can decide whether to fall back or
// surface the original message.
func runOverpassQuery(ctx context.Context, endpoint, query string, categories []CategoryID) ([]POI, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader("data="+url.QueryEscape(query)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// overpass-api.de runs Apache with mod_negotiation; go's http.DefaultClient
	// sends "Accept: */*" which triggers a 406 from its content type handler.
	// [out:json] makes Overpass emit application/json, so advertise it.
	req.Header.Set("Accept", "application/json, application/osm3s+xml;q=0.8")
	// Both overpass-api.de and other public mirrors require a meaningful
	// User-Agent string to avoid blanket rate-limiting (HTTP 429). Operators
	// can override this through the POI_PROVIDER_USER_AGENT env var; the
	// default below just identifies the software without claiming any host URL.
	req.Header.Set("User-Agent", firstNonEmpty(os.Getenv("POI_PROVIDER_USER_AGENT"), "media-library-self-hosted POI client"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("overpass returned HTTP %d", resp.StatusCode)
	}
	var raw overpassResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("overpass returned invalid JSON")
	}
	// tag→category map for reverse assignment
	tagCat := map[string]CategoryID{
		"restaurant": CategoryFood, "cafe": CategoryFood, "fast_food": CategoryFood, "pub": CategoryFood, "bar": CategoryFood,
		"fuel": CategoryFuelParking, "charging_station": CategoryFuelParking, "parking": CategoryFuelParking,
		"hotel": CategoryLodging, "hostel": CategoryLodging, "motel": CategoryLodging, "camp_site": CategoryLodging,
		"attraction": CategoryAttraction, "museum": CategoryAttraction, "viewpoint": CategoryAttraction, "park": CategoryAttraction, "waterfall": CategoryAttraction,
		"pharmacy": CategoryHealth, "hospital": CategoryHealth, "dentist": CategoryHealth, "clinic": CategoryHealth, "doctors": CategoryHealth, "atm": CategoryHealth, "bank": CategoryHealth,
		"supermarket": CategoryShops, "convenience": CategoryShops, "mall": CategoryShops,
	}
	out := make([]POI, 0, len(raw.Elements))
	for _, el := range raw.Elements {
		lat, lon := el.Lat, el.Lon
		if lat == 0 && lon == 0 && el.Center != nil {
			lat, lon = el.Center.Lat, el.Center.Lon
		}
		if lat == 0 && lon == 0 {
			continue
		}
		cat := classifyForCategory(el.Tags, categories, tagCat)
		if cat == "" {
			continue
		}
		p := POI{
			ID:       fmt.Sprintf("%s/%d", el.Type, el.ID),
			Name:     el.Tags["name"],
			Category: cat,
			Lat:      lat,
			Lon:      lon,
			Website:  el.Tags["website"],
			ImageURL: el.Tags["image"],
		}
		if v := el.Tags["wikipedia"]; v != "" {
			// "de:Some_City_Hotel" → "Some_City_Hotel" (strip lang prefix)
			if idx := strings.IndexByte(v, ':'); idx > 0 {
				v = v[idx+1:]
			}
			p.WikipediaTitle = strings.ReplaceAll(v, " ", "_")
		}
		out = append(out, p)
	}
	return out, nil
}

// classifyForCategory maps an OSM tag set to one of the enabled categories.
func classifyForCategory(tags map[string]string, enabled []CategoryID, tagCat map[string]CategoryID) CategoryID {
	keys := []string{"amenity", "tourism", "shop", "leisure", "natural", "craft", "office"}
	for _, key := range keys {
		if v := tags[key]; v != "" {
			if cat, ok := tagCat[v]; ok {
				for _, c := range enabled {
					if c == cat {
						return cat
					}
				}
			}
		}
	}
	return ""
}

// ── Geoapify ─────────────────────────────────────────────────────────────────

// geoapifyCategories maps our CategoryIDs to Geoapify category strings.
var geoapifyCategories = map[CategoryID]string{
	CategoryFood:        "catering.restaurant,catering.cafe,catering.fast_food,catering.pub",
	CategoryFuelParking: "service.vehicle.fuel,parking",
	CategoryLodging:     "accommodation.hotel,accommodation.motel,accommodation.hostel,camping",
	CategoryAttraction:  "entertainment.museum,entertainment.attraction,leisure.park,tourism.viewpoint",
	CategoryHealth:      "healthcare.clinic,healthcare.pharmacy,commercial.bank,commercial.atm",
	CategoryShops:       "commercial.supermarket,commercial.convenience,commercial.shopping_mall",
}

type geoapifyResponse struct {
	Features []struct {
		Properties struct {
			Name       string            `json:"name"`
			Lat        float64           `json:"lat"`
			Lon        float64           `json:"lon"`
			Website    string            `json:"website"`
			Categories []string          `json:"categories"`
			Datasource struct {
				Raw map[string]any `json:"raw"`
			} `json:"datasource"`
		} `json:"properties"`
	} `json:"features"`
}

func fetchGeoapify(ctx context.Context, apiKey string, bbox BBox, categories []CategoryID) ([]POI, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("poi: geoapify requires an API key")
	}
	// build category query: union of all enabled categories
	var cats []string
	for _, c := range categories {
		if v, ok := geoapifyCategories[c]; ok {
			cats = append(cats, strings.Split(v, ",")...)
		}
	}
	filter := fmt.Sprintf("rect:%f,%f,%f,%f", bbox.West, bbox.South, bbox.East, bbox.North)
	apiURL := fmt.Sprintf("https://api.geoapify.com/v2/places?categories=%s&filter=%s&limit=50&format=json&apiKey=%s",
		url.QueryEscape(strings.Join(cats, ",")), url.QueryEscape(filter), url.QueryEscape(apiKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("poi: geoapify %d: %s", resp.StatusCode, string(b))
	}
	var raw geoapifyResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("poi: geoapify decode: %w", err)
	}
	out := make([]POI, 0, len(raw.Features))
	for i, f := range raw.Features {
		p := f.Properties
		if p.Name == "" {
			continue
		}
		// determine category from first matched geoapify category string
		cat := matchGeoapifyCategory(p.Categories, categories)
		if cat == "" {
			cat = categories[0] // fallback: first enabled category
		}
		poi := POI{
			ID:       fmt.Sprintf("gp/%d", i),
			Name:     p.Name,
			Category: cat,
			Lat:      p.Lat,
			Lon:      p.Lon,
			Website:  p.Website,
		}
		if wd, ok := p.Datasource.Raw["wikidata"].(string); ok && wd != "" {
			poi.WikipediaTitle = wd // client can fetch image from wikidata via Wikimedia API
		}
		out = append(out, poi)
	}
	return out, nil
}

func matchGeoapifyCategory(geoCats []string, enabled []CategoryID) CategoryID {
	for _, gc := range geoCats {
		switch {
		case strings.HasPrefix(gc, "catering"):
			if containsCat(enabled, CategoryFood) { return CategoryFood }
		case strings.HasPrefix(gc, "service.vehicle") || strings.HasPrefix(gc, "parking"):
			if containsCat(enabled, CategoryFuelParking) { return CategoryFuelParking }
		case strings.HasPrefix(gc, "accommodation") || strings.HasPrefix(gc, "camping"):
			if containsCat(enabled, CategoryLodging) { return CategoryLodging }
		case strings.HasPrefix(gc, "entertainment") || strings.HasPrefix(gc, "leisure") || strings.HasPrefix(gc, "tourism"):
			if containsCat(enabled, CategoryAttraction) { return CategoryAttraction }
		case strings.HasPrefix(gc, "healthcare") || strings.HasPrefix(gc, "commercial.bank") || strings.HasPrefix(gc, "commercial.atm"):
			if containsCat(enabled, CategoryHealth) { return CategoryHealth }
		case strings.HasPrefix(gc, "commercial.supermarket") || strings.HasPrefix(gc, "commercial.convenience") || strings.HasPrefix(gc, "commercial.shopping"):
			if containsCat(enabled, CategoryShops) { return CategoryShops }
		}
	}
	return ""
}

func containsCat(cats []CategoryID, target CategoryID) bool {
	for _, c := range cats {
		if c == target {
			return true
		}
	}
	return false
}

// ── Mapbox ───────────────────────────────────────────────────────────────────

type mapboxResponse struct {
	Features []struct {
		Geometry struct {
			Coordinates [2]float64 `json:"coordinates"`
		} `json:"geometry"`
		Properties struct {
			Name      string `json:"name"`
			MapboxID  string `json:"mapbox_id"`
			WikiDataID string `json:"wikidata"`
			Context   struct {
				Poi struct {
					Category string `json:"category"`
				} `json:"poi"`
			} `json:"context"`
			RichData struct {
				WebsiteURL string `json:"website"`
			} `json:"rich_data"`
		} `json:"properties"`
	} `json:"features"`
}

// mapboxSearchTerms maps categories to query terms for Mapbox Search.
var mapboxSearchTerms = map[CategoryID]string{
	CategoryFood:        "restaurant cafe fast food pub",
	CategoryFuelParking: "fuel gas station parking",
	CategoryLodging:     "hotel hostel motel",
	CategoryAttraction:  "museum attraction viewpoint park",
	CategoryHealth:      "pharmacy hospital dentist bank atm",
	CategoryShops:       "supermarket convenience store shopping mall",
}

func fetchMapbox(ctx context.Context, token string, bbox BBox, categories []CategoryID) ([]POI, error) {
	if token == "" {
		return nil, fmt.Errorf("poi: mapbox requires an access token")
	}
	seen := map[string]bool{}
	var out []POI
	for _, cat := range categories {
		terms := mapboxSearchTerms[cat]
		for _, term := range strings.Fields(terms) {
			apiURL := fmt.Sprintf("https://api.mapbox.com/search/geocode/v6/forward?q=%s&bbox=%f,%f,%f,%f&access_token=%s&limit=5",
				url.QueryEscape(term), bbox.West, bbox.South, bbox.East, bbox.North, url.QueryEscape(token))
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
			if err != nil {
				return nil, err
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return nil, err
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				continue
			}
			var raw mapboxResponse
			err = json.NewDecoder(resp.Body).Decode(&raw)
			resp.Body.Close()
			if err != nil {
				continue
			}
			for _, f := range raw.Features {
				p := f.Properties
				id := p.MapboxID
				if id == "" {
					id = fmt.Sprintf("mb/%s/%s", term, p.Name)
				}
				if seen[id] || p.Name == "" {
					continue
				}
				seen[id] = true
				// map context category to ours
				catID := mapboxCategoryToCategory(p.Context.Poi.Category, cat)
				out = append(out, POI{
					ID:             id,
					Name:           p.Name,
					Category:       catID,
					Lat:            f.Geometry.Coordinates[1],
					Lon:            f.Geometry.Coordinates[0],
					Website:        p.RichData.WebsiteURL,
					WikipediaTitle: p.WikiDataID,
				})
			}
		}
	}
	return out, nil
}

func mapboxCategoryToCategory(mbCat string, fallback CategoryID) CategoryID {
	switch {
	case strings.Contains(mbCat, "restaurant") || strings.Contains(mbCat, "cafe") || strings.Contains(mbCat, "bar"):
		return CategoryFood
	case strings.Contains(mbCat, "fuel") || strings.Contains(mbCat, "parking"):
		return CategoryFuelParking
	case strings.Contains(mbCat, "hotel") || strings.Contains(mbCat, "hostel") || strings.Contains(mbCat, "lodging"):
		return CategoryLodging
	case strings.Contains(mbCat, "museum") || strings.Contains(mbCat, "attraction") || strings.Contains(mbCat, "landmark"):
		return CategoryAttraction
	case strings.Contains(mbCat, "pharmacy") || strings.Contains(mbCat, "hospital") || strings.Contains(mbCat, "bank"):
		return CategoryHealth
	case strings.Contains(mbCat, "supermarket") || strings.Contains(mbCat, "store") || strings.Contains(mbCat, "mall"):
		return CategoryShops
	default:
		return fallback
	}
}

// ── Cache ────────────────────────────────────────────────────────────────────

type cacheKey struct {
	provider string
	keyHash  string // hash of provider config
	bbox     string
	cats     string
}

type cacheEntry struct {
	val       []POI
	expiresAt time.Time
}

var (
	cacheMu sync.Mutex
	cache   = map[cacheKey]*cacheEntry{}
)

const cacheTTL = 5 * time.Minute

func cacheKeyFor(provider, key string, bbox BBox, cats []CategoryID) cacheKey {
	catParts := make([]string, len(cats))
	for i, c := range cats {
		catParts[i] = string(c)
	}
	return cacheKey{
		provider: provider,
		keyHash:  key[:min(len(key), 32)],
		bbox:     fmt.Sprintf("%f,%f,%f,%f", bbox.West, bbox.South, bbox.East, bbox.North),
		cats:     strings.Join(catParts, ","),
	}
}

func cacheGet(provider, key string, bbox BBox, cats []CategoryID) ([]POI, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	e, ok := cache[cacheKeyFor(provider, key, bbox, cats)]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	out := make([]POI, len(e.val))
	copy(out, e.val)
	return out, true
}

func cacheSet(provider, key string, bbox BBox, cats []CategoryID, val []POI) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cache[cacheKeyFor(provider, key, bbox, cats)] = &cacheEntry{val: val, expiresAt: time.Now().Add(cacheTTL)}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}