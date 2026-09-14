package poi

import (
	"reflect"
	"strings"
	"testing"
)

// goldenExpansions pins the exact category → provider expansion used to build
// provider requests. These strings are validated remotely by the providers and
// there is no machine-readable category list to read from (Overpass is raw OSM
// tags, Geoapify/Mapbox only document their identifiers on docs pages). Any
// change to a mapping below is therefore an intentional product decision and
// must be reviewed explicitly rather than silently drifting.
var goldenExpansions = map[string]struct {
	overpass []string
	geoapify string
	mapbox   string
}{
	"food": {
		overpass: []string{`"amenity"~"restaurant|cafe|fast_food|pub|bar"`},
		geoapify: "catering.restaurant,catering.cafe,catering.fast_food,catering.pub",
		mapbox:   "restaurant cafe fast food pub",
	},
	"fuel_parking": {
		overpass: []string{`"amenity"~"fuel|charging_station|parking"`},
		geoapify: "service.vehicle.fuel,parking",
		mapbox:   "fuel gas station parking",
	},
	"lodging": {
		overpass: []string{`"tourism"~"hotel|hostel|motel|camp_site"`},
		geoapify: "accommodation.hotel,accommodation.motel,accommodation.hostel,camping",
		mapbox:   "hotel hostel motel",
	},
	"attraction": {
		overpass: []string{`"tourism"~"attraction|museum|viewpoint"`, `"leisure"="park"`, `"natural"="waterfall"`},
		geoapify: "entertainment.museum,tourism.attraction,leisure.park,tourism.viewpoint",
		mapbox:   "museum attraction viewpoint park",
	},
	"health": {
		overpass: []string{`"amenity"~"pharmacy|hospital|dentist|clinic|doctors|atm|bank"`},
		geoapify: "healthcare.clinic,healthcare.pharmacy,commercial.bank,commercial.atm",
		mapbox:   "pharmacy hospital dentist bank atm",
	},
	"shops": {
		overpass: []string{`"shop"~"supermarket|convenience|mall"`},
		geoapify: "commercial.supermarket,commercial.convenience,commercial.shopping_mall",
		mapbox:   "supermarket convenience store shopping mall",
	},
}

func TestExpansionMapsMatchGoldenData(t *testing.T) {
	for id, expected := range goldenExpansions {
		cat := CategoryID(id)
		if !ValidCategories[cat] {
			t.Errorf("%s is not registered in ValidCategories", id)
			continue
		}
		if got := overpassConditions(cat); !reflect.DeepEqual(got, expected.overpass) {
			t.Errorf("overpassConditions(%q) = %#v, want %#v", id, got, expected.overpass)
		}
		if got := geoapifyCategories[cat]; got != expected.geoapify {
			t.Errorf("geoapifyCategories[%q] = %q, want %q", id, got, expected.geoapify)
		}
		if got := mapboxSearchTerms[cat]; got != expected.mapbox {
			t.Errorf("mapboxSearchTerms[%q] = %q, want %q", id, got, expected.mapbox)
		}
	}
	for cat := range ValidCategories {
		if _, ok := goldenExpansions[string(cat)]; !ok {
			t.Errorf("category %q has no golden expansion; add one to keep provider mappings pinned", cat)
		}
		if overpassConditions(cat) == nil {
			t.Errorf("category %q has no overpass conditions", cat)
		}
		if geoapifyCategories[cat] == "" {
			t.Errorf("category %q has no geoapify categories", cat)
		}
		if mapboxSearchTerms[cat] == "" {
			t.Errorf("category %q has no mapbox search terms", cat)
		}
	}
}

func TestGeoapifyCategoriesNeverCarryStaleIdentifiers(t *testing.T) {
	// "entertainment.attraction" is not a Geoapify category (the live id is
	// "tourism.attraction"); a regression here would surface as HTTP 400 from
	// api.geoapify.com.
	for cat, values := range geoapifyCategories {
		if strings.Contains(values, "entertainment.attraction") {
			t.Errorf("category %q expands to %q which contains the unsupported Geoapify identifier", cat, values)
		}
		for _, id := range strings.Split(values, ",") {
			if id == "" {
				t.Errorf("category %q has an empty Geoapify identifier", cat)
			}
		}
	}
}