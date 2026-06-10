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
