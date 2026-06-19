package promptcompat

import (
	"bytes"
	"encoding/json"
)

// OrderedJSONMap 是一个保留插入顺序的 JSON 对象包装器。
//
// 标准 map[string]any 在 json.Marshal 时会按 key 字典序输出，无法对齐真实
// Android App 抓包观察到的字段顺序。本类型实现 json.Marshaler 接口，按
// Order 中的插入顺序序列化字段；同时保留 map 风格的 Get/Set/Delete 访问。
//
// 使用方式：
//
//	m := NewOrderedJSONMap()
//	m.Set("chat_session_id", "abc")
//	m.Set("prompt", "你好")
//	raw, _ := json.Marshal(m) // 按插入顺序输出
type OrderedJSONMap struct {
	M     map[string]any
	Order []string
}

// NewOrderedJSONMap 创建一个空的 OrderedJSONMap。
func NewOrderedJSONMap() *OrderedJSONMap {
	return &OrderedJSONMap{
		M:     make(map[string]any),
		Order: make([]string, 0),
	}
}

// Set 写入键值；新增 key 时追加到 Order 末尾，已存在 key 时仅更新值。
func (m *OrderedJSONMap) Set(key string, val any) {
	if m == nil {
		return
	}
	if _, exists := m.M[key]; !exists {
		m.Order = append(m.Order, key)
	}
	m.M[key] = val
}

// Get 读取键值。
func (m *OrderedJSONMap) Get(key string) (any, bool) {
	if m == nil {
		return nil, false
	}
	v, ok := m.M[key]
	return v, ok
}

// Delete 删除键值，同时从 Order 中移除。
func (m *OrderedJSONMap) Delete(key string) {
	if m == nil {
		return
	}
	if _, exists := m.M[key]; !exists {
		return
	}
	delete(m.M, key)
	for i, k := range m.Order {
		if k == key {
			m.Order = append(m.Order[:i], m.Order[i+1:]...)
			break
		}
	}
}

// Len 返回字段数量。
func (m *OrderedJSONMap) Len() int {
	if m == nil {
		return 0
	}
	return len(m.Order)
}

// MarshalJSON 按 Order 顺序序列化为 JSON 对象。
// 未知 key（不在 Order 中但存在于 M）不会输出，避免数据漂移。
func (m *OrderedJSONMap) MarshalJSON() ([]byte, error) {
	if m == nil || len(m.Order) == 0 {
		return []byte("{}"), nil
	}
	var buf bytes.Buffer
	buf.Grow(64)
	buf.WriteByte('{')
	for i, k := range m.Order {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		vb, err := json.Marshal(m.M[k])
		if err != nil {
			return nil, err
		}
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// AsMap 返回底层的 map[string]any 视图，供需要按 key 读取的旧调用方使用。
// 注意：通过此 map 视图读取是安全的，但若直接对返回值做 json.Marshal，
// 将退化为按 key 字典序输出（无法保留顺序），需调用方自行注意。
func (m *OrderedJSONMap) AsMap() map[string]any {
	if m == nil {
		return nil
	}
	return m.M
}
