# ginx

[![CI](https://github.com/gtkit/ginx/actions/workflows/ci.yml/badge.svg)](https://github.com/gtkit/ginx/actions/workflows/ci.yml)

`github.com/gtkit/ginx` 提供一组与业务无关的通用 [gin](https://github.com/gin-gonic/gin) 请求参数处理工具，
聚焦"gin 原生没有优雅解、又跨服务复用"的请求侧痛点：

- **body 只能读一次** —— 读取后回填，使后续 `ShouldBind` 仍可完整读取
- **原始 body 获取** —— 读取、缓存并回填完整字节，webhook 验签后仍可绑定
- **按 Content-Type 解析 body** —— `application/json` 与 `application/x-www-form-urlencoded`
- **只绑 body、不混入 query** —— 规避 gin form 模式把 URL query 并入绑定；支持基于缓存的重复绑定
- **单值 header / query 校验** —— 去空、查重，防御 HTTP 参数污染
- **泛型类型化取值** —— query / 路由参数直接取 int、bool 等，缺失或非法回退默认值
- **Content-Type 白名单** —— 解析与绑定前快速拒绝，便于返回 415
- **请求 body 硬限长** —— 基于 `http.MaxBytesReader`，防御超大 body 撑爆内存

本包不包含任何中间件、响应封装或业务语义；中间件、统一响应等请放在各自的包中。

## 安装

```bash
go get github.com/gtkit/ginx
```

要求 Go 1.26+。

## API

| 函数 / 类型 | 说明 |
|---|---|
| `ParseBody(c) BodySources` | 解析并按请求缓存 body，返回 `Available` / `JSON` / `Form` / `Err` |
| `BodyString(c, field) string` | 从 body 取字段并转字符串（JSON 标量或 form 值） |
| `BindBody(c, obj) error` | 只把 body 绑定到 obj，不信任 query |
| `RawBody(c) ([]byte, error)` | 读取完整原始 body，缓存并回填（webhook 验签） |
| `BindBodyCached(c, obj) error` | 基于缓存字节绑定，同一请求可重复调用 |
| `RequireContentType(c, types...) error` | Content-Type 白名单校验 |
| `SingleValueHeader(c, key) (string, error)` | 读取并校验单值 header |
| `SingleValueQuery(c, key) (string, error)` | 读取并校验单值 query（防参数污染） |
| `Query[T](c, key, def) T` / `Param[T](c, key, def) T` | 泛型类型化取值，缺失/非法回退默认值 |
| `LimitRequestBody(c, maxBytes)` | 为 body 设置硬上限（`http.MaxBytesReader`） |
| `IsRequestBodyTooLarge(err) bool` | 判断 err 是否因 body 超过硬上限产生 |
| `MaxBodyBytes` | `ParseBody` 的解析软上限（8 KiB） |
| `ErrDuplicateHeader` / `ErrInvalidHeaderValue` / `ErrInvalidBindContext` | header / 绑定哨兵错误 |
| `ErrNoBody` / `ErrBodyTooLarge` / `ErrUnsupportedContentType` / `ErrMalformedBody` | `ParseBody` 失败原因哨兵错误（经 `BodySources.Err` 透出） |

## 用法

### 解析 body（多来源、按请求缓存）

```go
src := ginx.ParseBody(c)
if !src.Available {
    // 失败原因可判定：ErrNoBody / ErrBodyTooLarge / ErrUnsupportedContentType / ErrMalformedBody
    if errors.Is(src.Err, ginx.ErrBodyTooLarge) {
        // body 超过 MaxBodyBytes 软上限
    }
    return
}
name, _ := src.JSON["name"].(string) // JSON body（数字为 json.Number，原文不丢精度）
page := src.Form.Get("page")         // form body
```

### 只绑 body + 硬限长

```go
ginx.LimitRequestBody(c, 1<<20) // 1 MiB 硬上限

var req CreateOrderReq
if err := ginx.BindBody(c, &req); err != nil {
    if ginx.IsRequestBodyTooLarge(err) {
        c.AbortWithStatus(http.StatusRequestEntityTooLarge) // 413
        return
    }
    c.AbortWithStatus(http.StatusBadRequest)
    return
}
```

### webhook 验签：原始 body + 重复绑定

```go
ginx.LimitRequestBody(c, 1<<20) // RawBody 不设软上限，务必配合硬限长

raw, err := ginx.RawBody(c) // 完整原始字节，读后回填并缓存
if err != nil { /* ... */ }
verifySignature(raw, c.GetHeader("X-Signature"))

var notify PayNotify
_ = ginx.BindBodyCached(c, &notify) // 可重复绑定，每次都是完整 body
```

### 类型化取参数

```go
page := ginx.Query(c, "page", 1)        // ?page=3 -> 3；缺失/非法 -> 1
dry  := ginx.Query(c, "dry_run", false)
id   := ginx.Param(c, "id", int64(0))   // 路由 /user/:id

uid, err := ginx.SingleValueQuery(c, "uid") // ?uid=1&uid=2 -> ErrDuplicateQuery
```

### Content-Type 白名单

```go
if err := ginx.RequireContentType(c, "application/json"); err != nil {
    c.AbortWithStatus(http.StatusUnsupportedMediaType) // 415
    return
}
```

### 单值 header

```go
token, err := ginx.SingleValueHeader(c, "X-Token")
switch {
case errors.Is(err, ginx.ErrDuplicateHeader):
    // 同名头出现多个值
case errors.Is(err, ginx.ErrInvalidHeaderValue):
    // 值含逗号等非法形式
}
```

## 软限长 vs 硬限长

- `MaxBodyBytes`（软）：仅约束 `ParseBody` 自身读取/解析的长度，超过则返回 `Available=false`，**不影响**下游 `ShouldBind`。
- `LimitRequestBody`（硬）：包装 `c.Request.Body`，对整个请求生命周期内的任何读取生效，超限即返回 `*http.MaxBytesError`。

二者互补：`ParseBody` 用于"轻量探取 body 中的字段"，`LimitRequestBody` 用于"防御性地封顶请求体"。

## 使用约束

- **并发**：与 gin.Context 一致，本包函数会替换 `c.Request.Body`、临时修改 `URL.RawQuery`，仅限在处理该请求的 handler goroutine 内调用
- **multipart/form-data 不支持**：multipart 通常携带文件、体积大，与按 `MaxBodyBytes` 轻量探取的定位冲突
- **`BindBody` 一次性语义**：绑定会消费 body 流，同一请求内二次调用读到空 body；如需先探取字段再绑定，先 `ParseBody`（会回填 body）再 `BindBody`
- **JSON 数字**：以 `json.Number` 原文承载，`BodyString` 对任意大小的整数不丢精度（如雪花 ID）
