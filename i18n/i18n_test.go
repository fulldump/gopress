package i18n

import "testing"

func TestNormalize(t *testing.T) {
	tests := map[string]Locale{
		"es": Spanish, "ES-mx": Spanish,
		"en": English, "en_US": English,
		"zh": Chinese, "zh-CN": Chinese,
		"fr": "", "": "",
	}
	for input, expected := range tests {
		if actual := Normalize(input); actual != expected {
			t.Errorf("Normalize(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestCatalogsHaveMatchingKeys(t *testing.T) {
	for locale, messages := range catalogs {
		for key := range spanishMessages {
			if messages[key] == "" {
				t.Errorf("catalog %q is missing %q", locale, key)
			}
		}
		if len(messages) != len(spanishMessages) {
			t.Errorf("catalog %q has %d messages, want %d", locale, len(messages), len(spanishMessages))
		}
	}
}
