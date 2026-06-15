package ginx

import (
	"errors"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ErrDuplicateQuery 表示同名 query 参数出现了多个非空值（HTTP 参数污染）。
var ErrDuplicateQuery = errors.New("query parameter provided multiple times")

// Scalar 是 Query 与 Param 支持的取值类型集合。约束使用精确类型（不带 ~）以保证零反射解析；
// 命名类型（如 type ID int64）请按底层类型取值后自行转换。
type Scalar interface {
	string | int | int64 | uint64 | bool | float64
}

// SingleValueQuery 读取并校验单值 query 参数：去空白后忽略空值，出现多个非空值视为参数污染、
// 返回 ErrDuplicateQuery。恰好一个非空值时返回该值，全部为空时返回空串且无错误。query 值中的
// 逗号视为合法（逗号合并是 header 的语义，query 没有）。context、Request 或 URL 为 nil 时
// 返回空串且无错误。
func SingleValueQuery(c *gin.Context, key string) (string, error) {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return "", nil
	}
	var single string
	count := 0
	for _, value := range c.Request.URL.Query()[key] {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		count++
		if count > 1 {
			return "", ErrDuplicateQuery
		}
		single = value
	}
	return single, nil
}

// Query 从 query 参数读取 key 并解析为 T：值缺失、去空白后为空、或解析失败时返回 def，
// 不 panic、不返回错误。需要严格校验时请使用 gin 的 ShouldBindQuery 配合 binding tag。
func Query[T Scalar](c *gin.Context, key string, def T) T {
	if c == nil || c.Request == nil {
		return def
	}
	return parseScalar(c.Query(key), def)
}

// Param 从路由参数（如 /user/:id 的 id）读取 key 并解析为 T，缺失与解析失败的回退语义同 Query。
func Param[T Scalar](c *gin.Context, key string, def T) T {
	if c == nil {
		return def
	}
	return parseScalar(c.Param(key), def)
}

// Body 从请求 body 中读取指定 field 并解析为 T。按 JSON → Form 的顺序查找 field。
// 值缺失、类型不匹配、body 不可解析（含 GET/HEAD、Content-Type 不匹配等）时返回 def，
// 不 panic、不返回错误。需要严格校验时请先 ParseBody 再自行处理。
//
// 本函数通过 ParseBody 缓存，同一请求内多次调用或与 BodyString 混用只解析一次 body。
func Body[T Scalar](c *gin.Context, field string, def T) T {
	sources := ParseBody(c)
	if !sources.Available {
		return def
	}
	if v := jsonScalarString(sources.JSON[field]); v != "" {
		return parseScalar(v, def)
	}
	return parseScalar(sources.Form.Get(field), def)
}

// QuerySlice 从 query 参数读取 key 的多个值并返回类型 T 的切片。
// 例如 ?id=1&id=2&id=3 可经 QuerySlice[int64](c, "id") 取得 []int64{1,2,3}。
// 非空字符串总是有效；非 string 类型的值中解析失败的条目被静默跳过。
// key 不存在、context 为 nil 或无有效值时返回 nil。
func QuerySlice[T Scalar](c *gin.Context, key string) []T {
	if c == nil || c.Request == nil {
		return nil
	}
	values, ok := c.Request.URL.Query()[key]
	if !ok || len(values) == 0 {
		return nil
	}
	result := make([]T, 0, len(values))
	for _, v := range values {
		if p, ok := parseScalarValue[T](v); ok {
			result = append(result, p)
		}
	}
	return result
}

// parseScalarValue 将去空白后的 s 解析为 T 并报告是否成功；s 为空时直接返回失败。
func parseScalarValue[T Scalar](s string) (T, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return *new(T), false
	}
	var out T
	switch p := any(&out).(type) {
	case *string:
		*p = s
		return out, true
	case *int:
		v, err := strconv.Atoi(s)
		if err != nil {
			return out, false
		}
		*p = v
	case *int64:
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return out, false
		}
		*p = v
	case *uint64:
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return out, false
		}
		*p = v
	case *bool:
		v, err := strconv.ParseBool(s)
		if err != nil {
			return out, false
		}
		*p = v
	case *float64:
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return out, false
		}
		*p = v
	}
	return out, true
}

// parseScalar 将去空白后的 s 解析为 T；s 为空或解析失败时返回 def。
func parseScalar[T Scalar](s string, def T) T {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	var out T
	switch p := any(&out).(type) {
	case *string:
		*p = s
	case *int:
		v, err := strconv.Atoi(s)
		if err != nil {
			return def
		}
		*p = v
	case *int64:
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return def
		}
		*p = v
	case *uint64:
		v, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return def
		}
		*p = v
	case *bool:
		v, err := strconv.ParseBool(s)
		if err != nil {
			return def
		}
		*p = v
	case *float64:
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return def
		}
		*p = v
	}
	return out
}
