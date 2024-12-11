package api

import (
	"time"
)

type File struct {
	Id string `json:"id"` // todo: this is part of persistence layer/logic

	AuthorId      string `json:"author_id"`
	AuthorNick    string `json:"author_nick"`
	AuthorPicture string `json:"author_picture"`

	Name string `json:"name"`
	Size int64  `json:"size"`
	Mime string `json:"mime"`

	CreatedOn time.Time `json:"createdOn"`
}
