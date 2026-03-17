package maps

func GetString(m map[string]any, key, def string) string {
	if m == nil {
		return def
	}
	value, ok := m[key]
	if !ok {
		return def
	}
	str, ok := value.(string)
	if !ok {
		return def
	}

	return str
}

func GetStringSlice(m map[string]any, key string) []string {
	def := make([]string, 0)
	if m == nil {
		return def
	}
	value, ok := m[key]
	if !ok {
		return def
	}
	ms, ok := value.([]string)
	if ok {
		return ms
	}
	mi, ok := value.([]any)
	if !ok {
		return def
	}

	for i := range mi {
		m2, ok := mi[i].(string)
		if !ok {
			continue
		}
		def = append(def, m2)
	}

	return def
}

func GetMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	value, ok := m[key]
	if !ok {
		return map[string]any{}
	}
	sub, ok := value.(map[string]any)
	if !ok {
		return map[string]any{}
	}

	return sub
}

func GetUint64(m map[string]any, key string) uint64 {
	if m == nil {
		return 0
	}
	value, ok := m[key]
	if !ok {
		return 0
	}
	u64, ok := value.(uint64)
	if ok {
		return u64
	}
	u, ok := value.(uint)
	if ok {
		return uint64(u)
	}
	i64, ok := value.(int64)
	if ok {
		return uint64(i64)
	}
	i, ok := value.(int)
	if ok {
		return uint64(i)
	}

	return 0
}
