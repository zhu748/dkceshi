package toolcall

import "github.com/santhosh-tekuri/jsonschema/v6"

// Small request-local cache: reuse compilation/fingerprinting for repeated
// tools without allocating another large index for mostly distinct catalogs.
// The shared bounded cache remains authoritative for evicted entries.
type normalizationSchemaCache struct {
	entries [8]struct {
		name   string
		schema *jsonschema.Schema
	}
	count, next int
}

func (c *normalizationSchemaCache) get(name string, raw any) *jsonschema.Schema {
	if raw == nil {
		return nil
	}
	for _, entry := range c.entries[:c.count] {
		if entry.name == name {
			return entry.schema
		}
	}
	schema, _ := compileToolSchema(raw)
	c.entries[c.next].name, c.entries[c.next].schema = name, schema
	c.next = (c.next + 1) % len(c.entries)
	if c.count < len(c.entries) {
		c.count++
	}
	return schema
}
