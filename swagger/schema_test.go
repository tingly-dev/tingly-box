package swagger

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func newTestManager(t *testing.T) *RouteManager {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rm := NewRouteManager(gin.New())
	rm.SetSwaggerInfo(SwaggerInfo{
		Title:   "Test API",
		Version: "1.0.0",
	})
	return rm
}

// Generation must be deterministic: repeated generation of the same routes
// (including the tags section) yields byte-identical output.
func TestGenerateOpenAPIDeterministic(t *testing.T) {
	rm := newTestManager(t)
	group := rm.NewGroup("api", "v1", "")

	handler := func(c *gin.Context) {}
	tags := []string{"zeta", "alpha", "mid", "beta", "omega", "kappa", "iota"}
	for _, tag := range tags {
		group.GET("/"+tag, handler, WithDescription(tag), WithTags(tag))
	}

	first, err := rm.GenerateOpenAPI(VersionV3)
	assert.NoError(t, err)
	for i := 0; i < 5; i++ {
		next, err := rm.GenerateOpenAPI(VersionV3)
		assert.NoError(t, err)
		assert.Equal(t, first, next, "generation %d differs", i)
	}

	// Tags must be sorted by name
	var spec OpenAPI
	assert.NoError(t, json.Unmarshal([]byte(first), &spec))
	for i := 1; i < len(spec.Tags); i++ {
		assert.LessOrEqual(t, spec.Tags[i-1].Name, spec.Tags[i].Name)
	}
}

// Fields tagged json:"-" never appear in marshaled JSON and must not appear
// in the schema either.
func TestSchemaSkipsJSONDashFields(t *testing.T) {
	type Model struct {
		Visible string `json:"visible"`
		Hidden  string `json:"-"`
		secret  string //lint:ignore U1000 unexported fields must be skipped
	}
	_ = Model{}.secret

	schema := newSchemaGen(VersionV3).modelSchema(Model{})
	assert.Contains(t, schema.Properties, "visible")
	assert.NotContains(t, schema.Properties, "Hidden")
	assert.NotContains(t, schema.Properties, "secret")
	assert.NotContains(t, schema.Required, "Hidden")
}

// Embedded structs without a json name are flattened into the parent,
// matching encoding/json marshaling.
func TestSchemaFlattensEmbeddedStructs(t *testing.T) {
	type Base struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at,omitempty"`
	}
	type PtrBase struct {
		Extra string `json:"extra"`
	}
	type Model struct {
		Base
		*PtrBase
		Name string `json:"name"`
	}

	gen := newSchemaGen(VersionV3)
	schema := gen.modelSchema(Model{})

	assert.Contains(t, schema.Properties, "id")
	assert.Contains(t, schema.Properties, "created_at")
	assert.Contains(t, schema.Properties, "extra")
	assert.Contains(t, schema.Properties, "name")
	assert.NotContains(t, schema.Properties, "Base")
	assert.NotContains(t, schema.Properties, "PtrBase")
	// Flattened required semantics carry over
	assert.Contains(t, schema.Required, "id")
	assert.NotContains(t, schema.Required, "created_at")
	// The embedded types are not emitted as standalone definitions
	assert.NotContains(t, gen.collected, "Base")
	assert.NotContains(t, gen.collected, "PtrBase")
}

// []byte marshals as a base64 string; json.RawMessage is arbitrary JSON.
func TestSchemaByteSliceAndRawMessage(t *testing.T) {
	type Model struct {
		Blob []byte          `json:"blob"`
		Raw  json.RawMessage `json:"raw"`
	}

	schema := newSchemaGen(VersionV3).modelSchema(Model{})

	blob := schema.Properties["blob"]
	assert.Equal(t, "string", blob.Type)
	assert.Equal(t, "byte", blob.Format)

	raw := schema.Properties["raw"]
	assert.Empty(t, raw.Type)
	assert.Empty(t, raw.Ref)
	assert.Nil(t, raw.Items)
}

// Map values that are anonymous structs must be inlined, not emitted as a
// dangling empty $ref.
func TestSchemaMapOfAnonymousStruct(t *testing.T) {
	type Model struct {
		Data map[string]struct {
			Value string `json:"value"`
		} `json:"data"`
	}

	schema := newSchemaGen(VersionV3).modelSchema(Model{})
	data := schema.Properties["data"]
	assert.Equal(t, "object", data.Type)
	if assert.NotNil(t, data.AdditionalProperties) {
		assert.Empty(t, data.AdditionalProperties.Ref)
		assert.Contains(t, data.AdditionalProperties.Properties, "value")
	}
}

// Named structs reachable only through maps, slices, or deep nesting must all
// end up in the definitions.
func TestBuildDefinitionsCollectsDeepModels(t *testing.T) {
	type Leaf struct {
		Value string `json:"value"`
	}
	type Mid struct {
		Leaves map[string]Leaf `json:"leaves"`
	}
	type Root struct {
		Mids []*Mid `json:"mids"`
	}

	defs := newSchemaGen(VersionV3).buildDefinitions(map[string]interface{}{"Root": Root{}})
	assert.Contains(t, defs, "Root")
	assert.Contains(t, defs, "Mid")
	assert.Contains(t, defs, "Leaf")
}

// Self-referencing models must not hang generation.
func TestBuildDefinitionsHandlesRecursiveModels(t *testing.T) {
	type Node struct {
		Name     string  `json:"name"`
		Children []*Node `json:"children,omitempty"`
	}

	defs := newSchemaGen(VersionV3).buildDefinitions(map[string]interface{}{"Node": Node{}})
	assert.Contains(t, defs, "Node")
	children := defs["Node"].Properties["children"]
	assert.Equal(t, "array", children.Type)
	assert.Equal(t, "#/components/schemas/Node", children.Items.Ref)
}

// Binding tags on a nested-struct field must not attach validation keywords
// as $ref siblings (forbidden in OpenAPI 3.0).
func TestRefFieldsCarryNoValidationSiblings(t *testing.T) {
	type Inner struct {
		Value string `json:"value"`
	}
	type Model struct {
		Inner Inner `json:"inner" binding:"required" description:"should not appear"`
	}

	schema := newSchemaGen(VersionV3).modelSchema(Model{})
	inner := schema.Properties["inner"]
	assert.NotEmpty(t, inner.Ref)
	assert.Empty(t, inner.Description)
	assert.Empty(t, inner.Type)
	assert.Nil(t, inner.Minimum)
	assert.Contains(t, schema.Required, "inner")
}

// len= on strings constrains length; on slices it constrains item count.
func TestValidationRulesLenAndBounds(t *testing.T) {
	type Model struct {
		Code  string   `json:"code" binding:"len=6"`
		Items []string `json:"items" binding:"min=1,max=5"`
		Score float64  `json:"score" binding:"gt=0,lte=100"`
	}

	schema := newSchemaGen(VersionV3).modelSchema(Model{})

	code := schema.Properties["code"]
	if assert.NotNil(t, code.MinLength) && assert.NotNil(t, code.MaxLength) {
		assert.Equal(t, 6, *code.MinLength)
		assert.Equal(t, 6, *code.MaxLength)
	}

	items := schema.Properties["items"]
	if assert.NotNil(t, items.MinItems) && assert.NotNil(t, items.MaxItems) {
		assert.Equal(t, 1, *items.MinItems)
		assert.Equal(t, 5, *items.MaxItems)
	}

	score := schema.Properties["score"]
	if assert.NotNil(t, score.Minimum) && assert.NotNil(t, score.Maximum) {
		assert.Equal(t, 0.0, *score.Minimum)
		assert.True(t, score.ExclusiveMinimum)
		assert.Equal(t, 100.0, *score.Maximum)
		assert.False(t, score.ExclusiveMaximum)
	}
}

// Query parameter names follow gin's binding: form tag wins over json tag,
// and "-" opts the field out.
func TestModelQueryParams(t *testing.T) {
	type Query struct {
		Page     int    `form:"page" json:"page_json" binding:"required"`
		Name     string `json:"name"`
		Ignored  string `form:"-"`
		Excluded string `json:"-"`
	}

	params := modelQueryParams(Query{})
	names := make(map[string]queryParamSpec, len(params))
	for _, p := range params {
		names[p.Name] = p
	}

	assert.Contains(t, names, "page")
	assert.True(t, names["page"].Required)
	assert.Equal(t, "integer", names["page"].Type)
	assert.Contains(t, names, "name")
	assert.NotContains(t, names, "page_json")
	assert.NotContains(t, names, "Ignored")
	assert.NotContains(t, names, "Excluded")
}

// AddMiddleware must register each middleware exactly once, even when called
// multiple times.
func TestAddMiddlewareRegistersOnce(t *testing.T) {
	rm := newTestManager(t)
	group := rm.NewGroup("api", "v1", "")

	calls := map[string]int{}
	counter := func(name string) gin.HandlerFunc {
		return func(c *gin.Context) {
			calls[name]++
			c.Next()
		}
	}
	group.AddMiddleware(counter("first"))
	group.AddMiddleware(counter("second"))

	group.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	rm.GetEngine().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, calls["first"], "first middleware ran %d times", calls["first"])
	assert.Equal(t, 1, calls["second"], "second middleware ran %d times", calls["second"])
}
