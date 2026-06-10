# Changelog

本项目遵循 [Keep a Changelog 1.1.0](https://keepachangelog.com/zh-CN/1.1.0/)。

## [Unreleased]

### Added

- `BodySources` 新增 `Err` 字段，`ParseBody` 失败原因可经 `errors.Is` 判定；新增哨兵错误 `ErrNoBody` / `ErrBodyTooLarge` / `ErrUnsupportedContentType` / `ErrMalformedBody`

### Fixed

- 修复 `BodyString` 读取超过 2^53 的 JSON 整数时精度丢失的问题；JSON 数字现按 body 原文返回（如 `1.50` 返回 `"1.50"` 而非 `"1.5"`）

## [1.0.1] - 2026-06-01

无对外功能变更，与 v1.0.0 内容一致。

## [1.0.0] - 2026-06-01

### Added

- 首个版本：从业务项目下沉的通用 gin 请求处理能力
  - `ParseBody` / `BodySources`：按请求缓存的多来源 body 解析（JSON / form）
  - `BodyString`：从 body 取字段并转字符串
  - `BindBody`：只绑 body、不混入 query
  - `SingleValueHeader`：单值 header 校验
  - `LimitRequestBody` / `IsRequestBodyTooLarge`：基于 `http.MaxBytesReader` 的请求体硬限长
  - `MaxBodyBytes` 软上限常量与 `ErrDuplicateHeader` / `ErrInvalidHeaderValue` / `ErrInvalidBindContext` 哨兵错误

[Unreleased]: https://github.com/gtkit/ginx/compare/v1.0.1...HEAD
[1.0.1]: https://github.com/gtkit/ginx/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/gtkit/ginx/releases/tag/v1.0.0
