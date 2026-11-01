package oidc

import (
	"reflect"
	"strings"
	"testing"

	"github.com/teamhanko/hanko/backend/v3/config"
)

// Our provider config lives at the top-level "oidc" key, which Hanko ignores today. Fail loudly
// if a Hanko upgrade starts using that key itself.
func TestHankoConfigHasNoOIDCKey(t *testing.T) {
	var walk func(reflect.Type)
	walk = func(rt reflect.Type) {
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			tag := strings.Split(f.Tag.Get("koanf"), ",")[0]
			if tag == "" && f.Anonymous {
				walk(f.Type)
				continue
			}
			if strings.EqualFold(tag, "oidc") {
				t.Fatalf("Hanko's config now has a top-level %q key (field %s); move our provider config", tag, f.Name)
			}
		}
	}
	walk(reflect.TypeOf(config.Config{}))
}
