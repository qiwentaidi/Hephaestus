package theme

import (
	"reflect"
	"strings"
	"testing"
)

func validDefinition() Definition {
	return Definition{
		SchemaVersion: 1,
		Base:          "dark",
		Radius:        "0.5rem",
		Colors: Colors{
			Background:            "#101114",
			Foreground:            "#ECECF1",
			Card:                  "#181a20",
			Primary:               "#5b8def",
			PrimaryForeground:     "#ffffff",
			Muted:                 "#1c1f26",
			MutedForeground:       "#a0a7b4",
			Border:                "#2a2f3a",
			Sidebar:               "#0c0d10",
			SidebarForeground:     "#d7dae2",
			Destructive:           "#e5484d",
			DestructiveForeground: "#ffffff",
			Ring:                  "#5b8def",
		},
	}
}

func TestNormalizeName(t *testing.T) {
	name, key, err := NormalizeName("  Ocean   Blue  ")
	if err != nil {
		t.Fatal(err)
	}
	if name != "Ocean Blue" || key != "ocean blue" {
		t.Fatalf("unexpected normalized name %q / key %q", name, key)
	}

	if _, _, err := NormalizeName("   "); err == nil {
		t.Fatal("expected empty name to be rejected")
	}
	if _, _, err := NormalizeName(strings.Repeat("a", MaxNameRunes+1)); err == nil {
		t.Fatal("expected overlong name to be rejected")
	}
}

func TestNormalizeDefinition(t *testing.T) {
	def, err := NormalizeDefinition(validDefinition())
	if err != nil {
		t.Fatal(err)
	}
	if def.Colors.Background != "#101114" || def.Base != "dark" {
		t.Fatalf("unexpected normalized definition: %+v", def)
	}

	cases := []struct {
		name   string
		mutate func(*Definition)
	}{
		{"bad base", func(d *Definition) { d.Base = "sepia" }},
		{"bad color", func(d *Definition) { d.Colors.Primary = "red" }},
		{"css injection", func(d *Definition) { d.Colors.Primary = "#fff; background:url(x)" }},
		{"bad radius", func(d *Definition) { d.Radius = "calc(1rem)" }},
		{"bad version", func(d *Definition) { d.SchemaVersion = 2 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := validDefinition()
			tc.mutate(&def)
			if _, err := NormalizeDefinition(def); err == nil {
				t.Fatal("expected invalid definition to be rejected")
			}
		})
	}
}

func TestNormalizeDefinitionOverrides(t *testing.T) {
	def := validDefinition()
	def.Overrides = map[string]string{"--card": "#123456", "--primary": "#ABCDEF"}
	normalized, err := NormalizeDefinition(def)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Overrides["--card"] != "#123456" || normalized.Overrides["--primary"] != "#abcdef" {
		t.Fatalf("unexpected overrides: %+v", normalized.Overrides)
	}

	for _, overrides := range []map[string]string{
		{"--radius": "#ffffff"},
		{"--card": "red"},
		{"not-a-token": "#ffffff"},
	} {
		invalid := validDefinition()
		invalid.Overrides = overrides
		if _, err := NormalizeDefinition(invalid); err == nil {
			t.Fatalf("expected override %+v to be rejected", overrides)
		}
	}
}

func TestNormalizeDefinitionDefaultsStatusColors(t *testing.T) {
	def := validDefinition()
	def.Base = "light"
	normalized, err := NormalizeDefinition(def)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Colors.Success != "#18794e" || normalized.Colors.Warning != "#946800" || normalized.Colors.Info != "#0b6bcb" {
		t.Fatalf("unexpected light status defaults: %+v", normalized.Colors)
	}
}

func TestEncodeDecodeDefinition(t *testing.T) {
	def, err := NormalizeDefinition(validDefinition())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeDefinition(def)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, def) {
		t.Fatalf("round trip mismatch: %+v != %+v", decoded, def)
	}
}
