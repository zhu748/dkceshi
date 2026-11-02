package toolcall

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Schemas resolve only resources already supplied in this schema.
type localSchemaLoader struct{}

func (localSchemaLoader) Load(string) (any, error) {
	return nil, fmt.Errorf("external JSON Schema references are not supported")
}

type schemaBuild struct {
	done   chan struct{}
	schema *jsonschema.Schema
	err    error
}

var compiledSchemas = struct {
	sync.Mutex
	entries map[[32]byte]*jsonschema.Schema
	pending map[[32]byte]*schemaBuild
	order   [][32]byte
}{entries: map[[32]byte]*jsonschema.Schema{}, pending: map[[32]byte]*schemaBuild{}}

func compileToolSchema(raw any) (*jsonschema.Schema, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256(data)
	compiledSchemas.Lock()
	if cached := compiledSchemas.entries[key]; cached != nil {
		compiledSchemas.Unlock()
		return cached, nil
	}
	if build := compiledSchemas.pending[key]; build != nil {
		compiledSchemas.Unlock()
		<-build.done
		return build.schema, build.err
	}
	build := &schemaBuild{done: make(chan struct{})}
	compiledSchemas.pending[key] = build
	compiledSchemas.Unlock()
	build.schema, build.err = compileSchemaBytes(data)
	compiledSchemas.Lock()
	// Keep the existing cache size bounds; coalesce large concurrent builds too.
	if build.err == nil && len(data) <= 65536 {
		if len(compiledSchemas.order) >= 128 {
			delete(compiledSchemas.entries, compiledSchemas.order[0])
			compiledSchemas.order = compiledSchemas.order[1:]
		}
		compiledSchemas.entries[key] = build.schema
		compiledSchemas.order = append(compiledSchemas.order, key)
	}
	delete(compiledSchemas.pending, key)
	close(build.done)
	compiledSchemas.Unlock()
	return build.schema, build.err
}

func compileSchemaBytes(data []byte) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(localSchemaLoader{})
	const resource = "https://ds2api.invalid/tool-schema.json"
	if err = c.AddResource(resource, doc); err != nil {
		return nil, err
	}
	return c.Compile(resource)
}
