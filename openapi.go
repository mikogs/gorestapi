package gorestapi

import (
	"context"
	"encoding"
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mikogs/gorestapi/pkg/jsonresp"
)

// OpenAPIInfo describes the API in its OpenAPI document.
type OpenAPIInfo struct {
	Title       string
	Version     string
	Description string
	// Servers are the URLs of the API, with the prefix of its mount, e.g. https://api.example.com/api or /api.
	Servers []string
	// BearerFormat is the format of the tokens of Options.Authenticate, e.g. JWT, a hint for the clients.
	BearerFormat string
}

// OpenAPI returns the OpenAPI 3.0 document of the routes of the handler, as JSON: the operations that the routes allow,
// their parameters, e.g. the filters of the lists, the schemas of the structs of the records, with their validation
// rules, and the problems of their errors.
func (h *Handler) OpenAPI(info OpenAPIInfo) ([]byte, error) {
	doc := &openAPIDocument{
		OpenAPI: "3.0.3",
		Info:    openAPIInfo{Title: info.Title, Version: info.Version, Description: info.Description},
		Paths:   map[string]*openAPIPathItem{},
		Components: openAPIComponents{
			Schemas:   map[string]*openAPISchema{"Problem": problemSchema()},
			Responses: map[string]*openAPIResponse{},
		},
	}
	for _, server := range info.Servers {
		doc.Servers = append(doc.Servers, openAPIServer{URL: server})
	}
	// the tokens of the requests, which the handler reads only from the Authorization header, see Bearer
	if h.options.Authenticate != nil {
		doc.Components.SecuritySchemes = map[string]*openAPISecurityScheme{
			"bearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: info.BearerFormat},
		}
		doc.Security = []map[string][]string{{"bearerAuth": {}}}
	}
	schemas := &schemaBuilder{doc: doc, names: map[reflect.Type]string{}, reg: h.reg, passwords: map[reflect.Type][]string{}}

	urlPaths := make([]string, 0, len(h.options.Routes))
	for urlPath := range h.options.Routes {
		urlPaths = append(urlPaths, urlPath)
	}
	sort.Strings(urlPaths)
	for _, urlPath := range urlPaths {
		h.addRoute(doc, schemas, urlPath, h.options.Routes[urlPath])
	}
	return json.MarshalIndent(doc, "", "  ")
}

func (h *Handler) addRoute(doc *openAPIDocument, schemas *schemaBuilder, urlPath string, route Route) {
	key := routeKey(urlPath, route)
	constructor := func(override func() any) any {
		obj := h.reg.New(key)
		if override != nil {
			obj = override()
		}
		schemas.passwords[reflect.Indirect(reflect.ValueOf(obj)).Type()] = h.reg.PasswordFieldNames(key)
		return obj
	}
	tag := urlPath
	doc.Tags = append(doc.Tags, openAPITag{Name: tag})
	collection := &openAPIPathItem{}
	record := &openAPIPathItem{Parameters: []*openAPIParameter{{
		Name: "id", In: "path", Required: true, Description: "The ID of the record: a number, or a UUID",
		Schema: &openAPISchema{Type: "string"},
	}}}
	name := operationName(urlPath)

	if route.Flags&DisableList == 0 {
		obj := constructor(route.ListConstructor)
		item := schemas.ref(reflect.TypeOf(obj))
		collection.Get = &openAPIOperation{
			OperationID: "list" + name, Tags: []string{tag}, Summary: "List the records of " + urlPath,
			Parameters: h.listParameters(urlPath, route),
			Responses: withProblems(map[string]*openAPIResponse{
				"200": jsonResponse("A page of the records", dataSchema(&openAPISchema{Type: "array", Items: item}, true)),
			}, doc, http.StatusBadRequest, http.StatusInternalServerError),
		}
	}
	if route.Flags&DisableCreate == 0 {
		obj := constructor(route.CreateConstructor)
		collection.Put = &openAPIOperation{
			OperationID: "create" + name, Tags: []string{tag}, Summary: "Create a record of " + urlPath,
			RequestBody: jsonRequestBody(schemas.ref(reflect.TypeOf(obj))),
			Responses: withProblems(map[string]*openAPIResponse{
				"201": jsonResponse("The record is created, with the ID", dataSchema(idSchema(), false)),
			}, doc, http.StatusBadRequest, http.StatusForbidden, http.StatusConflict, http.StatusRequestEntityTooLarge,
				http.StatusUnsupportedMediaType, http.StatusInternalServerError),
		}
	}
	if route.Flags&DisableRead == 0 {
		obj := constructor(route.ReadConstructor)
		record.Get = &openAPIOperation{
			OperationID: "read" + name, Tags: []string{tag}, Summary: "Read a record of " + urlPath,
			Responses: withProblems(map[string]*openAPIResponse{
				"200": jsonResponse("The record", dataSchema(schemas.ref(reflect.TypeOf(obj)), false)),
			}, doc, http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError),
		}
	}
	if route.Flags&DisableUpdate == 0 {
		obj := constructor(route.UpdateConstructor)
		record.Put = &openAPIOperation{
			OperationID: "update" + name, Tags: []string{tag}, Summary: "Update a record of " + urlPath,
			RequestBody: jsonRequestBody(schemas.ref(reflect.TypeOf(obj))),
			Responses: withProblems(map[string]*openAPIResponse{
				"200": jsonResponse("The record is updated", dataSchema(idSchema(), false)),
			}, doc, http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusConflict,
				http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusInternalServerError),
		}
	}
	if route.Flags&DisableDelete == 0 {
		record.Delete = &openAPIOperation{
			OperationID: "delete" + name, Tags: []string{tag}, Summary: "Delete a record of " + urlPath,
			Responses: withProblems(map[string]*openAPIResponse{
				"200": jsonResponse("The record is deleted", dataSchema(nil, false)),
			}, doc, http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusInternalServerError),
		}
	}
	for _, operation := range []*openAPIOperation{collection.Get, collection.Put, record.Get, record.Put, record.Delete} {
		if operation == nil {
			continue
		}
		if h.options.RateLimiter != nil {
			withProblems(operation.Responses, doc, http.StatusTooManyRequests)
		}
		if h.options.Authenticate == nil {
			continue
		}
		withProblems(operation.Responses, doc, http.StatusUnauthorized)
		// the requests of an anonymous route can have a token, or none
		if route.Anonymous {
			operation.Security = []map[string][]string{{}, {"bearerAuth": {}}}
		}
	}
	if collection.Get != nil || collection.Put != nil {
		doc.Paths["/"+urlPath+"/"] = collection
	}
	if record.Get != nil || record.Put != nil || record.Delete != nil {
		doc.Paths["/"+urlPath+"/{id}"] = record
	}
}

// listParameters returns the parameters of the lists of a route: the pagination, the order and the filters of the
// fields that clients may use in filters, the same as the handler allows.
func (h *Handler) listParameters(urlPath string, route Route) []*openAPIParameter {
	key := routeKey(urlPath, route)
	responseFields := h.responseFields(key, route)
	allowed := responseFields
	if len(route.AllowedFilters) > 0 {
		allowed = map[string]bool{}
		for _, field := range route.AllowedFilters {
			if strings.Contains(field, ".") && !h.pathAllowed(context.Background(), key, field) {
				continue
			}
			allowed[field] = true
		}
	}
	orders := map[string]bool{}
	for field := range responseFields {
		orders[field] = true
	}
	for field := range allowed {
		orders[field] = true
	}

	maxLimit := 1000
	params := []*openAPIParameter{
		{Name: "limit", In: "query", Description: "The largest number of records of the page", Schema: &openAPISchema{Type: "integer", Minimum: ptr(1.0), Maximum: ptr(float64(maxLimit)), Default: 10}},
		{Name: "offset", In: "query", Description: "The number of records before the page", Schema: &openAPISchema{Type: "integer", Minimum: ptr(0.0), Default: 0}},
		{Name: "after", In: "query", Description: "The cursor of the next page, meta.next of the previous page, which cannot be combined with offset", Schema: &openAPISchema{Type: "string"}},
		{Name: "order", In: "query", Description: "The field to sort by", Schema: &openAPISchema{Type: "string", Enum: sortedEnum(orders)}},
		{Name: "order_direction", In: "query", Schema: &openAPISchema{Type: "string", Enum: []any{"asc", "desc"}}},
	}
	if route.Flags&DisableFilters != 0 {
		return params
	}
	ops := []any{"eq", "ne", "lt", "le", "gt", "ge", "like", "bit", "in"}
	if route.Flags&EnableMatchFilters != 0 {
		ops = append(ops, "match")
	}
	for _, field := range sortedKeys(allowed) {
		params = append(params,
			&openAPIParameter{Name: filterValPrefix + field, In: "query", Description: "The value of the filter of " + field +
				", converted to the type of the field; the values of in are separated with commas. A name with a group after a tilde, e.g. " +
				filterValPrefix + field + "~q, joins the filters of the group with OR", Schema: &openAPISchema{Type: "string"}},
			&openAPIParameter{Name: filterOpPrefix + field, In: "query", Description: "The operator of the filter of " + field, Schema: &openAPISchema{Type: "string", Enum: ops, Default: "eq"}},
		)
	}
	return params
}

// operationName returns the part of the names of the operations of a route, e.g. Notes for notes.
func operationName(urlPath string) string {
	var name strings.Builder
	for _, part := range strings.FieldsFunc(urlPath, func(r rune) bool { return r == '-' || r == '_' || r == '/' }) {
		name.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return name.String()
}

func jsonRequestBody(schema *openAPISchema) *openAPIRequestBody {
	return &openAPIRequestBody{Required: true, Content: map[string]openAPIMediaType{"application/json": {Schema: schema}}}
}

func jsonResponse(description string, schema *openAPISchema) *openAPIResponse {
	return &openAPIResponse{Description: description, Content: map[string]openAPIMediaType{"application/json": {Schema: schema}}}
}

// dataSchema returns the schema of a successful response, with data, and meta for a page of a list.
func dataSchema(data *openAPISchema, page bool) *openAPISchema {
	schema := &openAPISchema{Type: "object", Required: []string{"ok", "code"}, Properties: map[string]*openAPISchema{
		"ok":   {Type: "boolean"},
		"code": {Type: "string", Enum: []any{jsonresp.CodeSuccess, jsonresp.CodeCreated}},
	}}
	if data != nil {
		schema.Properties["data"] = data
	}
	if page {
		schema.Properties["meta"] = &openAPISchema{Type: "object", Properties: map[string]*openAPISchema{
			"next": {Type: "string", Description: "The cursor of the next page, for after, on a full page"},
		}}
	}
	return schema
}

func idSchema() *openAPISchema {
	return &openAPISchema{Type: "object", Properties: map[string]*openAPISchema{
		"id": {Description: "The ID of the record: a number, or a UUID", OneOf: []*openAPISchema{{Type: "integer", Minimum: ptr(0.0)}, {Type: "string", Format: "uuid"}}},
	}}
}

// withProblems adds the responses of the problems of statuses to responses, as references to the components of doc.
func withProblems(responses map[string]*openAPIResponse, doc *openAPIDocument, statuses ...int) map[string]*openAPIResponse {
	for _, status := range statuses {
		name := "Problem" + strconv.Itoa(status)
		if _, ok := doc.Components.Responses[name]; !ok {
			var codes []any
			for code, problemType := range problemTypes {
				if problemType.status == status {
					codes = append(codes, code)
				}
			}
			sort.Slice(codes, func(i, j int) bool { return codes[i].(string) < codes[j].(string) })
			schema := &openAPISchema{AllOf: []*openAPISchema{
				{Ref: "#/components/schemas/Problem"},
				{Type: "object", Properties: map[string]*openAPISchema{"code": {Type: "string", Enum: codes}}},
			}}
			doc.Components.Responses[name] = &openAPIResponse{
				Description: http.StatusText(status) + ", a problem of RFC 9457",
				Content:     map[string]openAPIMediaType{"application/problem+json": {Schema: schema}},
			}
		}
		responses[strconv.Itoa(status)] = &openAPIResponse{Ref: "#/components/responses/" + name}
	}
	return responses
}

func problemSchema() *openAPISchema {
	return &openAPISchema{
		Type:        "object",
		Description: "A problem of RFC 9457, Problem Details for HTTP APIs",
		Required:    []string{"type", "title", "status", "code"},
		Properties: map[string]*openAPISchema{
			"type":     {Type: "string", Description: "about:blank, or the URI of the type of the problem"},
			"title":    {Type: "string"},
			"status":   {Type: "integer"},
			"detail":   {Type: "string"},
			"instance": {Type: "string", Description: "The path of the request"},
			"code":     {Type: "string", Description: "The code of the problem"},
			"errors":   {Type: "object", Description: "The failed validation rules of the fields", AdditionalProperties: &openAPISchema{Type: "string"}},
		},
	}
}

// schemaBuilder builds the schemas of the structs of the records, as components of the document, by the JSON that
// encoding/json writes and reads for them.
type schemaBuilder struct {
	doc   *openAPIDocument
	names map[reflect.Type]string
	reg   Registry
	// passwords are the password fields of the structs of the routes, by the registry, e.g. with its tag name. The
	// password fields of other structs, e.g. related ones, are the fields with the pass option of the crud tag.
	passwords map[reflect.Type][]string
}

var (
	timeType          = reflect.TypeFor[time.Time]()
	textMarshalerType = reflect.TypeFor[encoding.TextMarshaler]()
	jsonMarshalerType = reflect.TypeFor[json.Marshaler]()
)

// ref returns the schema of t, a reference to the component of a struct.
func (b *schemaBuilder) ref(t reflect.Type) *openAPISchema {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	// a struct that writes its own JSON is not an object of its fields
	if t.Kind() != reflect.Struct || t == timeType || t.Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(jsonMarshalerType) ||
		t.Implements(textMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType) {
		return b.schema(t)
	}
	name, ok := b.names[t]
	if !ok {
		name = t.Name()
		for i := 2; b.doc.Components.Schemas[name] != nil; i++ {
			name = t.Name() + strconv.Itoa(i)
		}
		b.names[t] = name
		// the component exists before its fields, so that structs that reference each other end
		b.doc.Components.Schemas[name] = &openAPISchema{}
		*b.doc.Components.Schemas[name] = *b.structSchema(t)
	}
	return &openAPISchema{Ref: "#/components/schemas/" + name}
}

func (b *schemaBuilder) schema(t reflect.Type) *openAPISchema {
	switch {
	case t == timeType:
		return &openAPISchema{Type: "string", Format: "date-time"}
	case t.Kind() == reflect.Pointer:
		schema := b.schema(t.Elem())
		if schema.Ref != "" {
			return &openAPISchema{Nullable: true, AllOf: []*openAPISchema{schema}}
		}
		schema.Nullable = true
		return schema
	case t.Kind() == reflect.Array && t.Elem().Kind() == reflect.Uint8 && t.Len() == 16 && t.Implements(textMarshalerType):
		return &openAPISchema{Type: "string", Format: "uuid"}
	case t.Implements(jsonMarshalerType) || reflect.PointerTo(t).Implements(jsonMarshalerType):
		// a type that writes its own JSON, e.g. a decimal, can be anything
		return &openAPISchema{Description: "The JSON of " + t.String()}
	case t.Implements(textMarshalerType) || reflect.PointerTo(t).Implements(textMarshalerType):
		return &openAPISchema{Type: "string"}
	}
	switch t.Kind() {
	case reflect.Bool:
		return &openAPISchema{Type: "boolean"}
	case reflect.Int8, reflect.Int16, reflect.Int32:
		return &openAPISchema{Type: "integer", Format: "int32"}
	case reflect.Int, reflect.Int64:
		return &openAPISchema{Type: "integer", Format: "int64"}
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint, reflect.Uint64:
		return &openAPISchema{Type: "integer", Minimum: ptr(0.0)}
	case reflect.Float32:
		return &openAPISchema{Type: "number", Format: "float"}
	case reflect.Float64:
		return &openAPISchema{Type: "number", Format: "double"}
	case reflect.String:
		return &openAPISchema{Type: "string"}
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return &openAPISchema{Type: "string", Format: "byte"}
		}
		return &openAPISchema{Type: "array", Items: b.ref(t.Elem())}
	case reflect.Array:
		return &openAPISchema{Type: "array", Items: b.ref(t.Elem()), MinItems: ptr(t.Len()), MaxItems: ptr(t.Len())}
	case reflect.Map:
		return &openAPISchema{Type: "object", AdditionalProperties: b.ref(t.Elem())}
	case reflect.Struct:
		return b.ref(t)
	}
	return &openAPISchema{}
}

// structSchema returns the schema of the JSON object of a struct, with the rules of the validate and crud_regexp tags
// of its fields, the password fields as write only, and the fields set by the server as read only.
func (b *schemaBuilder) structSchema(t reflect.Type) *openAPISchema {
	schema := &openAPISchema{Type: "object", Properties: map[string]*openAPISchema{}}
	obj := reflect.New(t).Interface()
	passwords := map[string]bool{}
	if names, ok := b.passwords[t]; ok {
		for _, name := range names {
			passwords[name] = true
		}
	} else {
		for i := 0; i < t.NumField(); i++ {
			if strings.Contains(" "+t.Field(i).Tag.Get("crud")+" ", " pass ") {
				passwords[t.Field(i).Name] = true
			}
		}
	}
	serverFields := map[string]bool{"ID": true}
	if hasModificationFields(t) {
		for _, name := range []string{"CreatedAt", "CreatedBy", "ModifiedAt", "ModifiedBy"} {
			serverFields[name] = true
		}
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, omit, asString := jsonName(field)
		if omit || !field.IsExported() && !field.Anonymous {
			continue
		}
		if field.Anonymous && field.Tag.Get("json") == "" {
			embedded := field.Type
			for embedded.Kind() == reflect.Pointer {
				embedded = embedded.Elem()
			}
			if embedded.Kind() == reflect.Struct {
				inline := b.structSchema(embedded)
				for key, value := range inline.Properties {
					schema.Properties[key] = value
				}
				schema.Required = append(schema.Required, inline.Required...)
				continue
			}
		}

		var property *openAPISchema
		if related, ok := b.reg.RelatedType(obj, field.Name); ok {
			// the related records are not saved with the record, and are in its JSON only when they are loaded
			property = &openAPISchema{ReadOnly: true, Nullable: true, AllOf: []*openAPISchema{b.ref(related)}}
			if field.Type.Kind() == reflect.Slice {
				property = &openAPISchema{ReadOnly: true, Nullable: true, Type: "array", Items: b.ref(related)}
			}
		} else {
			property = b.schema(field.Type)
			if asString {
				property = &openAPISchema{Type: "string", Description: "A number as a string"}
			}
			if field.Name == "ID" && field.Type.Kind() == reflect.String {
				property.Format = "uuid"
			}
			if b.validationRules(property, field) {
				schema.Required = append(schema.Required, name)
			}
		}
		if passwords[field.Name] {
			property.WriteOnly = true
		}
		if serverFields[field.Name] {
			property.ReadOnly = true
		}
		schema.Properties[name] = property
	}
	sort.Strings(schema.Required)
	return schema
}

// validationRules adds the rules of the validate and crud_regexp tags of field to schema, and returns whether the field
// is required.
func (b *schemaBuilder) validationRules(schema *openAPISchema, field reflect.StructField) bool {
	if pattern := field.Tag.Get("crud_regexp"); pattern != "" {
		schema.Pattern = pattern
	}
	required := false
	base := field.Type
	for base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	isString := base.Kind() == reflect.String
	for _, rule := range strings.Split(field.Tag.Get("validate"), ",") {
		name, value, _ := strings.Cut(rule, "=")
		number, numberErr := strconv.ParseFloat(value, 64)
		switch {
		case name == "required":
			required = true
		case name == "email":
			schema.Format = "email"
		case name == "url" || name == "uri":
			schema.Format = "uri"
		case name == "uuid" || name == "uuid4" || name == "uuid7":
			schema.Format = "uuid"
		case name == "oneof":
			for _, option := range strings.Fields(value) {
				if isString {
					schema.Enum = append(schema.Enum, option)
				} else if n, err := strconv.ParseFloat(option, 64); err == nil {
					schema.Enum = append(schema.Enum, n)
				}
			}
		case numberErr != nil:
		case isString && (name == "min" || name == "gte"):
			schema.MinLength = ptr(int(number))
		case isString && (name == "max" || name == "lte"):
			schema.MaxLength = ptr(int(number))
		case isString && name == "len":
			schema.MinLength, schema.MaxLength = ptr(int(number)), ptr(int(number))
		case name == "min" || name == "gte":
			schema.Minimum = ptr(number)
		case name == "max" || name == "lte":
			schema.Maximum = ptr(number)
		case name == "gt":
			schema.Minimum, schema.ExclusiveMinimum = ptr(number), true
		case name == "lt":
			schema.Maximum, schema.ExclusiveMaximum = ptr(number), true
		}
	}
	return required
}

// jsonName returns the name of field in JSON, whether encoding/json leaves it out, and whether it writes it as a
// string, by its json tag.
func jsonName(field reflect.StructField) (string, bool, bool) {
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", true, false
	}
	name, options, _ := strings.Cut(tag, ",")
	if name == "" {
		name = field.Name
	}
	return name, false, strings.Contains(","+options+",", ",string,")
}

// hasModificationFields checks if t has the creation and modification fields that gocrud sets, see Struct of gocrud.
func hasModificationFields(t reflect.Type) bool {
	for _, name := range []string{"CreatedAt", "CreatedBy", "ModifiedAt", "ModifiedBy"} {
		if _, ok := t.FieldByName(name); !ok {
			return false
		}
	}
	return true
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedEnum(m map[string]bool) []any {
	var enum []any
	for _, key := range sortedKeys(m) {
		enum = append(enum, key)
	}
	return enum
}

func ptr[T any](value T) *T { return &value }

type openAPIDocument struct {
	OpenAPI    string                      `json:"openapi"`
	Info       openAPIInfo                 `json:"info"`
	Servers    []openAPIServer             `json:"servers,omitempty"`
	Tags       []openAPITag                `json:"tags,omitempty"`
	Security   []map[string][]string       `json:"security,omitempty"`
	Paths      map[string]*openAPIPathItem `json:"paths"`
	Components openAPIComponents           `json:"components"`
}

type openAPISecurityScheme struct {
	Type         string `json:"type"`
	Scheme       string `json:"scheme"`
	BearerFormat string `json:"bearerFormat,omitempty"`
}

type openAPIInfo struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
}

type openAPIServer struct {
	URL string `json:"url"`
}

type openAPITag struct {
	Name string `json:"name"`
}

type openAPIComponents struct {
	Schemas         map[string]*openAPISchema         `json:"schemas"`
	Responses       map[string]*openAPIResponse       `json:"responses"`
	SecuritySchemes map[string]*openAPISecurityScheme `json:"securitySchemes,omitempty"`
}

type openAPIPathItem struct {
	Parameters []*openAPIParameter `json:"parameters,omitempty"`
	Get        *openAPIOperation   `json:"get,omitempty"`
	Put        *openAPIOperation   `json:"put,omitempty"`
	Delete     *openAPIOperation   `json:"delete,omitempty"`
}

type openAPIOperation struct {
	OperationID string                      `json:"operationId"`
	Tags        []string                    `json:"tags,omitempty"`
	Summary     string                      `json:"summary,omitempty"`
	Parameters  []*openAPIParameter         `json:"parameters,omitempty"`
	RequestBody *openAPIRequestBody         `json:"requestBody,omitempty"`
	Security    []map[string][]string       `json:"security,omitempty"`
	Responses   map[string]*openAPIResponse `json:"responses"`
}

type openAPIParameter struct {
	Name        string         `json:"name"`
	In          string         `json:"in"`
	Description string         `json:"description,omitempty"`
	Required    bool           `json:"required,omitempty"`
	Schema      *openAPISchema `json:"schema"`
}

type openAPIRequestBody struct {
	Required bool                        `json:"required,omitempty"`
	Content  map[string]openAPIMediaType `json:"content"`
}

type openAPIResponse struct {
	Ref         string                      `json:"$ref,omitempty"`
	Description string                      `json:"description,omitempty"`
	Content     map[string]openAPIMediaType `json:"content,omitempty"`
}

type openAPIMediaType struct {
	Schema *openAPISchema `json:"schema"`
}

type openAPISchema struct {
	Ref                  string                    `json:"$ref,omitempty"`
	Type                 string                    `json:"type,omitempty"`
	Format               string                    `json:"format,omitempty"`
	Description          string                    `json:"description,omitempty"`
	Nullable             bool                      `json:"nullable,omitempty"`
	ReadOnly             bool                      `json:"readOnly,omitempty"`
	WriteOnly            bool                      `json:"writeOnly,omitempty"`
	Enum                 []any                     `json:"enum,omitempty"`
	Default              any                       `json:"default,omitempty"`
	Minimum              *float64                  `json:"minimum,omitempty"`
	Maximum              *float64                  `json:"maximum,omitempty"`
	ExclusiveMinimum     bool                      `json:"exclusiveMinimum,omitempty"`
	ExclusiveMaximum     bool                      `json:"exclusiveMaximum,omitempty"`
	MinLength            *int                      `json:"minLength,omitempty"`
	MaxLength            *int                      `json:"maxLength,omitempty"`
	MinItems             *int                      `json:"minItems,omitempty"`
	MaxItems             *int                      `json:"maxItems,omitempty"`
	Pattern              string                    `json:"pattern,omitempty"`
	Items                *openAPISchema            `json:"items,omitempty"`
	Properties           map[string]*openAPISchema `json:"properties,omitempty"`
	Required             []string                  `json:"required,omitempty"`
	AdditionalProperties *openAPISchema            `json:"additionalProperties,omitempty"`
	AllOf                []*openAPISchema          `json:"allOf,omitempty"`
	OneOf                []*openAPISchema          `json:"oneOf,omitempty"`
}
