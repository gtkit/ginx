# ginx

[![CI](https://github.com/gtkit/ginx/actions/workflows/ci.yml/badge.svg)](https://github.com/gtkit/ginx/actions/workflows/ci.yml)

`github.com/gtkit/ginx` 提供一组与业务无关的通用 [gin](https://github.com/gin-gonic/gin) 请求参数处理工具，
聚焦"gin 原生没有优雅解、又跨服务复用"的请求侧痛点：

- **body 只能读一次** —— 读取后回填，使后续 `ShouldBind` 仍可完整读取
- **原始 body 获取** —— 读取、缓存并回填完整字节，webhook 验签后仍可绑定
- **按 Content-Type 解析 body** —— `application/json` 与 `application/x-www-form-urlencoded`
- **只绑 body、不混入 query** —— 规避 gin form 模式把 URL query 并入绑定；支持基于缓存的重复绑定
- **单值 header / query 校验** —— 去空、查重，防御 HTTP 参数污染
- **泛型类型化取值** —— query / 路由参数 / body 字段 / header 直接取 int、bool 等类型
- **Content-Type 白名单** —— 解析与绑定前快速拒绝，便于返回 415
- **请求 body 硬限长** —— 基于 `http.MaxBytesReader`，防御超大 body 撑爆内存

本包不包含任何中间件、响应封装或业务语义；中间件、统一响应等请放在各自的包中。

### 为什么需要这个包？

gin 的 `ShouldBind`、`c.Query`、`c.Param` 等原生 API 在简单场景下足够好用，但在跨服务、生产级别的 Go 服务中，几个反复出现的痛点并没有原生解决：

**body 是一次性资源。** gin 的 body 读取流在第一个消费者消费后即关闭，后续调用 `ShouldBind` 读到空 body。本包在读取后自动回填：`ParseBody`、`RawBody` 读取后回填完整字节并按请求缓存，可重复调用、彼此自由组合；而 `BindBody` 直接消费 body 流（一次性语义），其后不应再假定 body 可读，需要重复绑定（如 webhook 先验签再绑定）请用 `BindBodyCached`。

**输入来源的信任边界需要显式划分。** gin 的 `ShouldBind` 在 form 模式下会合并 URL query 和 body，这在大多数安全敏感场景是不期望的——攻击者可以在 URL 中注入参数覆盖 body 中的字段。`BindBody` 通过临时清空 `RawQuery` 严格执行"只信 body、不信 query"原则。

**类型安全不应止步于字符串。** gin 的路由参数和 query 值都是字符串，业务代码中充斥着 `strconv.Atoi`、`strconv.ParseBool` 等样板转换。本包通过 Go 1.18+ 泛型提供 `Query[T]`、`Param[T]`、`Body[T]`、`Header[T]`，将字符串到目标类型的转换收敛在包内，调用方只需声明目标类型。

**HTTP 参数污染是真实攻击面。** 攻击者可以通过发送重复的 header 或 query 参数绕过单值校验逻辑。`SingleValueHeader` 和 `SingleValueQuery` 对此提供了显式防御。

**body 大小的防御应该是分层、可组合的。** 本包同时提供软上限（`MaxBodyBytes`，仅约束本包解析行为）和硬上限（`LimitRequestBody`，约束整个请求生命周期），二者互补而非替代，使用者可以根据安全等级灵活选择。

总之，ginx 不做 gin 已经做好的事，只填补 gin 原生 API 中那些"每个生产服务都要手写一遍"的空白——而且要求填得对、填得稳、填得零成本。

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
| `Query[T](c, key, def) T` / `Param[T](c, key, def) T` | 泛型类型化取值，缺失/非法回退默认值（宽松） |
| `QueryStrict[T](c, key) (T, error)` | 严格类型化取单值 query，区分缺失/重复/非法，便于返回 400 |
| `QuerySlice[T](c, key) []T` | 重复 query 参数的类型化切片提取（如 `?id=1&id=2`），无效值静默跳过 |
| `QuerySliceStrict[T](c, key) ([]T, error)` | 严格切片提取，遇任一非法值（含空值）报错，不静默跳过 |
| `Body[T](c, field, def) T` | 从 body（JSON / Form）取字段并类型化，通过 ParseBody 缓存 |
| `Header[T](c, key, def) T` | 读取单值 header 并类型化，重复/非法回退默认值 |
| `ContextValue[T](c, key) (T, bool)` | 从 gin.Context 键值存储中读取类型化值（中间件注入），类型不匹配返回 false |
| `LimitRequestBody(c, maxBytes)` | 为 body 设置硬上限（`http.MaxBytesReader`） |
| `IsRequestBodyTooLarge(err) bool` | 判断 err 是否因 body 超过硬上限产生 |
| `MaxBodyBytes` | `ParseBody` 的解析软上限（8 KiB） |
| `ContentTypeJSON` / `ContentTypeForm` | body Content-Type 常量，供 `RequireContentType` 等使用 |
| `ErrDuplicateQuery` / `ErrQueryMissing` / `ErrInvalidQueryValue` | `QueryStrict` / `QuerySliceStrict` 哨兵错误 |
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

> `Query[T]` 是宽松便利函数：同名参数出现多个值（`?id=1&id=2`）时静默取第一个，缺失/非法回退默认值，不报错。
> 需要严格区分缺失/重复/非法、或防御 HTTP 参数污染时，请改用 `QueryStrict` 或 `SingleValueQuery`。

需要明确报错（区分缺失 / 重复 / 非法，便于返回 400）时用严格版：

```go
id, err := ginx.QueryStrict[int64](c, "id")
switch {
case errors.Is(err, ginx.ErrQueryMissing):      // 参数缺失
case errors.Is(err, ginx.ErrDuplicateQuery):    // ?id=1&id=2 参数污染
case errors.Is(err, ginx.ErrInvalidQueryValue): // 无法解析为 int64
}

ids, err := ginx.QuerySliceStrict[int64](c, "id") // ?id=1&id=abc -> ErrInvalidQueryValue（不静默跳过）
```

### 重复 query 参数的类型化切片

```go
ids := ginx.QuerySlice[int64](c, "id")    // ?id=1&id=2&id=3 -> []int64{1,2,3}
names := ginx.QuerySlice[string](c, "name") // ?name=a&name=b -> ["a","b"]
```

解析失败的值静默跳过（如 `?id=1&id=abc&id=3` -> `[1, 3]`）。

### 从 Context 取类型化值

```go
// 中间件中注入
c.Set("user_id", uid)
c.Set("role", "admin")

// handler 中安全提取
userID, ok := ginx.ContextValue[int64](c, "user_id")
if !ok {
    // 未登录或类型异常
}
role, _ := ginx.ContextValue[string](c, "role")
```

消除逐次类型断言的样板代码，key 不存在或类型不匹配时返回 (zero, false)。

### Content-Type 白名单

```go
if err := ginx.RequireContentType(c, ginx.ContentTypeJSON); err != nil {
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

### 类型化取 body 字段

```go
count := ginx.Body(c, "count", 0)           // {"count":3} → 3；form 同理
name  := ginx.Body(c, "name", "")            // {"name":"alice"} → "alice"
ratio := ginx.Body(c, "ratio", 0.0)          // {"ratio":1.5} → 1.5
ok    := ginx.Body(c, "ok", false)           // {"ok":true} → true
```

缺失、类型不匹配、body 不可解析时一律回退默认值，不 panic。
通过 ParseBody 缓存，与 `BodyString` 混用也只解析一次 body。

### 类型化取 header

```go
page := ginx.Header(c, "X-Page", int64(0))  // X-Page: 42 → 42；缺失/重复 → 0
flag := ginx.Header(c, "X-Flag", false)      // X-Flag: true → true
```

继承 `SingleValueHeader` 的校验语义（去空/查重/防逗号），异常时回退默认值。
需要严格校验时请使用 `SingleValueHeader`。

## 软限长 vs 硬限长

- `MaxBodyBytes`（软）：仅约束 `ParseBody` 自身读取/解析的长度，超过则返回 `Available=false`，**不影响**下游 `ShouldBind`。
- `LimitRequestBody`（硬）：包装 `c.Request.Body`，对整个请求生命周期内的任何读取生效，超限即返回 `*http.MaxBytesError`。

二者互补：`ParseBody` 用于"轻量探取 body 中的字段"，`LimitRequestBody` 用于"防御性地封顶请求体"。

## 使用约束

- **并发**：与 gin.Context 一致，本包函数会替换 `c.Request.Body`、临时修改 `URL.RawQuery`，仅限在处理该请求的 handler goroutine 内调用
- **multipart/form-data 不支持**：multipart 通常携带文件、体积大，与按 `MaxBodyBytes` 轻量探取的定位冲突
- **`BindBody` 一次性语义**：绑定会消费 body 流，同一请求内二次调用读到空 body；如需先探取字段再绑定，先 `ParseBody`（会回填 body）再 `BindBody`
- **JSON 数字**：以 `json.Number` 原文承载，`BodyString` 对任意大小的整数不丢精度（如雪花 ID）
