# Binding — 请求绑定

HTTP 请求数据绑定与校验。

## 支持的绑定方式

| 绑定方式 | 方法 | Content-Type |
|---------------|--------|-------------|
| JSON | `BindJSON()` | `application/json` |
| XML | `BindXML()` | `application/xml` |
| Form | `BindForm()` | `multipart/form-data`、`form-urlencoded` |
| Query | `BindQuery()` | URL 查询参数 |
| Path | `BindPath()` | URL 路径参数 |
| Auto | `Bind()` | 自动推断 |