# ginx

[![CI](https://github.com/gtkit/ginx/actions/workflows/ci.yml/badge.svg)](https://github.com/gtkit/ginx/actions/workflows/ci.yml)

`github.com/gtkit/ginx` 提供一组与业务无关的通用 [gin](https://github.com/gin-gonic/gin) 请求参数处理工具，
聚焦"gin 原生没有优雅解、又跨服务复用"的请求侧痛点：

- **body 只能读一次** —— 读取后回填，使后续 `ShouldBind` 仍可完整读取
- **按 Content-Type 解析 body** —— `application/json` 与 `application/x-www-form-urlencoded`
- **只绑 body、不混入 query** —— 规避 gin form 模式把 URL query 并入绑定
- **单值 header 校验** —— 去空、查重、拒绝逗号多值
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
| `ParseBody(c) BodySources` | 解析并按请求缓存 body，返回 `Available` / `JSON` / `Form` |
| `BodyString(c, field) string` | 从 body 取字段并转字符串（JSON 标量或 form 值） |
| `BindBody(c, obj) error` | 只把 body 绑定到 obj，不信任 query |
| `SingleValueHeader(c, key) (string, error)` | 读取并校验单值 header |
| `LimitRequestBody(c, maxBytes)` | 为 body 设置硬上限（`http.MaxBytesReader`） |
| `IsRequestBodyTooLarge(err) bool` | 判断 err 是否因 body 超过硬上限产生 |
| `MaxBodyBytes` | `ParseBody` 的解析软上限（8 KiB） |
| `ErrDuplicateHeader` / `ErrInvalidHeaderValue` / `ErrInvalidBindContext` | 哨兵错误 |

## 用法

### 解析 body（多来源、按请求缓存）

```go
src := ginx.ParseBody(c)
if src.Available {
    name, _ := src.JSON["name"].(string) // JSON body
    page := src.Form.Get("page")         // form body
}
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
