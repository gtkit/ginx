# Changelog

本项目遵循 [Keep a Changelog 1.1.0](https://keepachangelog.com/zh-CN/1.1.0/)。

## [Unreleased]

### Added

- 首个版本：从业务项目下沉的通用 gin 请求处理能力
  - `ParseBody` / `BodySources`：按请求缓存的多来源 body 解析（JSON / form）
  - `BodyString`：从 body 取字段并转字符串
  - `BindBody`：只绑 body、不混入 query
  - `SingleValueHeader`：单值 header 校验
  - `LimitRequestBody` / `IsRequestBodyTooLarge`：基于 `http.MaxBytesReader` 的请求体硬限长
  - `MaxBodyBytes` 软上限常量与 `ErrDuplicateHeader` / `ErrInvalidHeaderValue` / `ErrInvalidBindContext` 哨兵错误
