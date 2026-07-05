package fields

import (
	"testing"

	"github.com/cyphix/gaighcli/internal/toon"
)

func TestParseFieldsUnknown(t *testing.T) {
	_, err := ParseFields("bad", map[string]ExtraFieldSpec{
		"foo": {JSONKey: "foo", Def: toon.Field("foo", "")},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseFieldsValid(t *testing.T) {
	result, err := ParseFields("foo", map[string]ExtraFieldSpec{
		"foo": {JSONKey: "foo", Def: toon.Field("foo", "")},
	})
	if err != nil || len(result.ExtraDefs) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
