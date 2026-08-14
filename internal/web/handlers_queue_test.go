package web

import (
	"net/url"
	"reflect"
	"testing"
)

func TestMetadataOverrides(t *testing.T) {
	cases := []struct {
		name string
		form url.Values
		path string
		want map[string]string
	}{
		{"none", url.Values{}, "Some Book", nil},
		{"one field", url.Values{
			"msrc:title:Some Book": {"audible"},
		}, "Some Book", map[string]string{"title": "audible"}},
		{"scoped to the item's path", url.Values{
			"msrc:title:Other Book": {"audible"},
		}, "Some Book", nil},
		{"multiple fields incl. composite keys", url.Values{
			"msrc:series:A/B":      {"audible"},
			"msrc:description:A/B": {"audnexus"},
			"chapters:A/B":         {"provider"}, // unrelated control ignored
		}, "A/B", map[string]string{"series": "audible", "description": "audnexus"}},
		{"unknown field names are not collected", url.Values{
			"msrc:bogus:Some Book": {"audible"},
		}, "Some Book", nil},
	}
	for _, c := range cases {
		if got := metadataOverrides(c.form, c.path); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: metadataOverrides = %v, want %v", c.name, got, c.want)
		}
	}
}
