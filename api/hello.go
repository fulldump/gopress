package api

import (
	"time"
)

var Hello = &Article{
	ArticleUserFields: ArticleUserFields{
		Title: "Primeros pasos",
		// 		Content: `
		// Bienvenido a GoPress.org, el sistema de blog gratuito escrito en golang y hackeable.
		//
		// Si estás leyendo este artículo es que ya has completado con éxito los siguientes pasos:
		//
		// 1. git clone https://github.com/fulldump/gopress.git
		// 2. cd gopress
		// 3. go run main.go
		// 4. open your browser on http://localhost:9955/hola
		//
		// A disfrutar!
		//
		// Recuerda desinstalar WordPress para liberar recursos y aumentar tu seguridad.
		// 	`,
	},
	CreatedOn: time.Date(2023, 04, 26, 00, 51, 00, 00, time.UTC),
	Published: true,
}
