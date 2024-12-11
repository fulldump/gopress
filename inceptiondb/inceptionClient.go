package inceptiondb

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
)

type Config struct {
	Base       string `json:"base"`
	DatabaseID string `json:"database_id"`
	ApiKey     string `json:"api_key"`
	ApiSecret  string `json:"api_secret"`
}

var DefaultHttpClient = &http.Client{} // TODO: tune client

type Client struct {
	config     Config
	HttpClient *http.Client
}

type JSON = map[string]interface{}

func NewClient(config Config) *Client {

	if config.DatabaseID != "" {
		config.Base += "/databases/" + url.PathEscape(config.DatabaseID)
	}

	return &Client{
		config:     config,
		HttpClient: DefaultHttpClient, // set default http client
	}
}

var ErrorAlreadyExist = errors.New("already exists")
var ErrorForbidden = errors.New("forbidden, your credentials does not have access")
var ErrorUnauthorized = errors.New("unauthorized, missing credentials")
var ErrorUnexpected = errors.New("unexpected error")

func (c *Client) CreateCollection(name string) error {
	endpoint := c.config.Base + "/collections"

	payload, err := json.Marshal(JSON{
		"name": name,
	}) // todo: handle err
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Api-Key", c.config.ApiKey)
	req.Header.Set("Api-Secret", c.config.ApiSecret)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode == http.StatusCreated {
		return nil
	}

	// body, _ := io.ReadAll(resp.Body)
	// log.Println("CreateCollection: " + string(body))

	if resp.StatusCode == http.StatusConflict {
		return ErrorAlreadyExist
	}
	if resp.StatusCode == http.StatusForbidden {
		return ErrorForbidden
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrorUnauthorized
	}

	return ErrorUnexpected
}

func (c *Client) DropCollection(name string) error {
	endpoint := c.config.Base + "/collections/" + url.PathEscape(name) + ":dropCollection"

	req, err := http.NewRequest("POST", endpoint, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Api-Key", c.config.ApiKey)
	req.Header.Set("Api-Secret", c.config.ApiSecret)

	_, err = c.HttpClient.Do(req)
	if err != nil {
		return err
	}

	return nil
}

func (c *Client) EnsureCollection(name string) error {

	c.CreateCollection(name)
	// todo: return nil only if conflict or created
	return nil
}

type IndexOptions struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	Field  string   `json:"field,omitempty"`
	Fields []string `json:"fields,omitempty"`
	Sparse bool     `json:"sparse,omitempty"`
	Unique bool     `json:"unique,omitempty"`
}

type ApiError struct {
	Description string `json:"description"`
	Message     string `json:"message"`
}

func (a *ApiError) Error() string {
	return a.Description + ": " + a.Message
}

func (a *ApiError) UnmarshalJSON(i []byte) error {

	d := struct {
		Error struct {
			Description string `json:"description"`
			Message     string `json:"message"`
		} `json:"error"`
	}{}

	err := json.Unmarshal(i, &d)
	if err != nil {
		return err
	}

	a.Description = d.Error.Description
	a.Message = d.Error.Message

	return nil
}

func (c *Client) CreateIndex(collection string, options *IndexOptions) error {

	endpoint := c.config.Base + "/collections/" + collection + ":createIndex"

	payload, err := json.Marshal(options)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	req.Header.Set("Api-Key", c.config.ApiKey)
	req.Header.Set("Api-Secret", c.config.ApiSecret)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		io.Copy(os.Stdout, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusCreated {
		return nil
	}

	apiError := ApiError{}
	json.NewDecoder(resp.Body).Decode(&apiError)

	if resp.StatusCode == http.StatusConflict || strings.Contains(apiError.Message, "already exists") {
		return ErrorAlreadyExist
	}
	if resp.StatusCode == http.StatusForbidden {
		return ErrorForbidden
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrorUnauthorized
	}

	return ErrorUnexpected
}

func (c *Client) EnsureIndex(collection string, options *IndexOptions) error {

	err := c.CreateIndex(collection, options)
	if err == ErrorAlreadyExist {
		return nil
	}

	return err
}

func (c *Client) Insert(collection string, document interface{}) error {

	payload, err := json.Marshal(document) // todo: handle err
	if err != nil {
		return err
	}

	endpoint := c.config.Base + "/collections/" + url.PathEscape(collection) + ":insert"
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Api-Key", c.config.ApiKey)
	req.Header.Set("Api-Secret", c.config.ApiSecret)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		io.Copy(os.Stdout, resp.Body)
		resp.Body.Close()
	}()

	// todo: unmarshal body on document?

	if resp.StatusCode == http.StatusCreated {
		return nil
	}

	apiError := ApiError{}
	json.NewDecoder(resp.Body).Decode(&apiError)

	if resp.StatusCode == http.StatusConflict || strings.Contains(apiError.Message, "index conflict") {
		return ErrorAlreadyExist
	}

	if resp.StatusCode == http.StatusForbidden {
		return ErrorForbidden
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return ErrorUnauthorized
	}

	return ErrorUnexpected
}

type FindQuery struct {
	Index   string `json:"index,omitempty"`
	Skip    int    `json:"skip,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Filter  JSON   `json:"filter,omitempty"`
	Reverse bool   `json:"reverse,omitempty"`
	From    JSON   `json:"from,omitempty"`
	To      JSON   `json:"to,omitempty"`
	Value   string `json:"value"`
}

func (c *Client) Find(collection string, query FindQuery) (io.ReadCloser, error) {

	payload, err := json.Marshal(query)
	if err != nil {
		return nil, err // todo: wrap error
	}

	endpoint := c.config.Base + "/collections/" + url.PathEscape(collection) + ":find"

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Api-Key", c.config.ApiKey)
	req.Header.Set("Api-Secret", c.config.ApiSecret)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("persistence read error")
	}

	if resp.StatusCode == http.StatusOK {
		return resp.Body, nil
	}

	defer func() {
		io.Copy(os.Stdout, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusForbidden {
		return nil, ErrorForbidden
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrorUnauthorized
	}

	return nil, ErrorUnexpected
}

func (c *Client) FindAll(collection string, query FindQuery, callback interface{}) error {

	t := reflect.ValueOf(callback)

	if t.Kind() != reflect.Func {
		return errors.New("expected to be a function")
	}

	if t.Type().NumIn() != 1 {
		return errors.New("function must have 1 argument")
	}

	arg0 := t.Type().In(0)
	if arg0.Kind() != reflect.Pointer {
		return errors.New("function argument must be pointer")
	}

	stream, err := c.Find(collection, query)
	if err != nil {
		return err
	}
	d := json.NewDecoder(stream)

	for {
		value := reflect.New(arg0.Elem())
		m := value.Interface()
		err := d.Decode(&m)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		t.Call([]reflect.Value{value})
	}

	return nil
}

func (c *Client) Remove(collection string, query FindQuery) (io.ReadCloser, error) {

	payload, err := json.Marshal(query)
	if err != nil {
		return nil, err // todo: wrap error
	}

	endpoint := c.config.Base + "/collections/" + url.PathEscape(collection) + ":remove"

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Api-Key", c.config.ApiKey)
	req.Header.Set("Api-Secret", c.config.ApiSecret)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("persistence read error")
	}

	if resp.StatusCode == http.StatusOK {
		return resp.Body, nil
	}

	defer func() {
		io.Copy(os.Stdout, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusForbidden {
		return nil, ErrorForbidden
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrorUnauthorized
	}

	return nil, ErrorUnexpected
}

type PatchQuery struct {
	Index   string `json:"index,omitempty"`
	Skip    int    `json:"skip,omitempty"`
	Limit   int    `json:"limit,omitempty"`
	Filter  JSON   `json:"filter,omitempty"`
	Reverse bool   `json:"reverse,omitempty"`
	From    JSON   `json:"from,omitempty"`
	To      JSON   `json:"to,omitempty"`
	Value   string `json:"value"`
	Patch   any    `json:"patch"`
}

func (c *Client) Patch(collection string, query PatchQuery) (io.ReadCloser, error) {

	payload, err := json.Marshal(query)
	if err != nil {
		return nil, err // todo: wrap error
	}

	endpoint := c.config.Base + "/collections/" + url.PathEscape(collection) + ":patch"

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Api-Key", c.config.ApiKey)
	req.Header.Set("Api-Secret", c.config.ApiSecret)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("persistence write error")
	}

	if resp.StatusCode == http.StatusOK {
		return resp.Body, nil
	}

	defer func() {
		io.Copy(os.Stdout, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusForbidden {
		return nil, ErrorForbidden
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrorUnauthorized
	}

	return nil, ErrorUnexpected
}

func (c *Client) FindOne(collection string, query FindQuery, item interface{}) error {

	query.Limit = 1

	r, err := c.Find(collection, query)
	if err != nil {
		return err
	}

	err = json.NewDecoder(r).Decode(&item)
	if err != nil {
		return err // todo: wrap? error decode: ?
	}
	io.Copy(io.Discard, r) // todo: handle err?
	r.Close()              // todo: handle err?

	return nil
}

type GetCollectionInfo struct {
	Name  string `json:"name"`
	Total int    `json:"total"`
}

func (c *Client) GetCollection(collection string) (*GetCollectionInfo, error) {

	endpoint := c.config.Base + "/collections/" + url.PathEscape(collection)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Api-Key", c.config.ApiKey)
	req.Header.Set("Api-Secret", c.config.ApiSecret)

	resp, err := c.HttpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("persistence read error")
	}

	if resp.StatusCode == http.StatusOK {
		output := &GetCollectionInfo{}
		err := json.NewDecoder(resp.Body).Decode(&output)
		if err != nil {
			return nil, err
		}
		return output, nil
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, ErrorForbidden
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrorUnauthorized
	}

	return nil, ErrorUnexpected
}
