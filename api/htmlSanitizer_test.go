package api

import (
	"testing"
)

func TestHtmlSanitizer(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "Remove tag <script>",
			input: `<script>alert(88)</script>Buenas tardes, me llamo Fulanez`,
			want:  `Buenas tardes, me llamo Fulanez`,
		},
		{
			name:  "Remove tag <script> (uppercase)",
			input: `<SCRIPT>alert(88)</scRipT>Buenas tardes, me llamo Fulanez`,
			want:  `Buenas tardes, me llamo Fulanez`,
		},
		{
			name:  "Remove tag <script> and leave allowed tags",
			input: `<script>alert(88)</script>Buenas <u>tardes</u>, me llamo Fulanez`,
			want:  `Buenas <u>tardes</u>, me llamo Fulanez`,
		},
		{
			name:  "Attribute starting by 'on*' are not allowed",
			input: `<p onclick="alert(1)">Texto</p>`,
			want:  `<p>Texto</p>`,
		},
		{
			name:  "Attribute starting by 'on*' are not allowed (upper)",
			input: `<p ONCLICK="alert(1)">Texto</p>`,
			want:  `<p>Texto</p>`,
		},
		{
			name:  "Etiqueta peligrosa dentro de un párrafo",
			input: `<a href="http://gopress.org/hello">Texto</p>`,
			want:  `<a href="http://gopress.org/hello">Texto</p>`,
		},
		{
			name:  "Attribute xlink:href for embedded SVGs",
			input: `<a xlink:href="http://gopress.org/hello">Text</p>`,
			want:  `<a>Text</p>`,
		},
		{
			name:  "Attribute style",
			input: `<div style="position: absolute;">Hello</div>`,
			want:  `<div>Hello</div>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			have := HtmlSanitizer(tt.input)
			if have != tt.want {
				t.Errorf("\nhave: %s\nwant: %s", have, tt.want)
			}
		})
	}
}
