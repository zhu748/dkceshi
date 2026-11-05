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

// Bound retained schema source bytes as well as entry count. Larger catalogs
// can now share compilation across normalization, validation and requests.
const schemaCacheByteLimit = 8 << 20
const schemaCacheEntryLimit = 1 << 20

var compiledSchemas = struct {
	sync.Mutex
	entries map[[32]byte]*jsonschema.Schema
	pending map[[32]byte]*schemaBuild
	order   [][32]byte
	sizes   map[[32]byte]int
	bytes   int
}{entries: map[[32]byte]*jsonschema.Schema{}, pending: map[[32]byte]*schemaBuild{}, sizes: map[[32]byte]int{}}

// CompileLocalJSONSchema shares the bounded, local-only compiler with response
// format validation. Neither tools nor output schemas may fetch external refs.
func CompileLocalJSONSchema(raw any) (*jsonschema.Schema, error) {
	return compileToolSchema(raw)
}

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
	if build.err == nil && len(data) <= schemaCacheEntryLimit {
		for len(compiledSchemas.order) >= 128 || compiledSchemas.bytes+len(data) > schemaCacheByteLimit {
			oldest := compiledSchemas.order[0]
			delete(compiledSchemas.entries, oldest)
			compiledSchemas.bytes -= compiledSchemas.sizes[oldest]
			delete(compiledSchemas.sizes, oldest)
			compiledSchemas.order = compiledSchemas.order[1:]
		}
		compiledSchemas.entries[key] = build.schema
		compiledSchemas.sizes[key] = len(data)
		compiledSchemas.bytes += len(data)
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
