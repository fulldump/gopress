package openapi

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/fulldump/box"
)

func newApiExample() *box.B {

	b := box.NewBox()

	b.Handle("GET", "/users", ListUsers)
	b.Handle("POST", "/users", CreateUser)

	b.Handle("GET", "/users/{userId}", GetUser)

	return b
}

type User struct {
	Id     string   `json:"id" description:"User identifier"`
	Name   string   `json:"name" description:"User name"`
	Tags   []string `json:"tags" description:"User tags"`
	Age    int      `json:"age" description:"User age"`
	Active bool     `json:"active" description:"User active"`
}

func ListUsers() []*User {
	return nil
}

type CreateUserInput struct {
	Id   string `json:"id" description:"If empty a random uuid will be generated"`
	Name string `json:"name"`
}

func CreateUser(input *CreateUserInput) *User {
	return &User{
		Id:   input.Id,
		Name: input.Name,
	}
}

func GetUser() *User {
	return nil
}

// https://editor-next.swagger.io/
func TestOpenApi(t *testing.T) {

	b := newApiExample()

	result := Spec(b)
	e := json.NewEncoder(os.Stdout)
	e.SetIndent("", "    ")
	e.Encode(result)
}
