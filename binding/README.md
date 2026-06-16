# Binding — Request Binding

HTTP request data binding and validation.

## Supported Binding Methods

| Binding Method | Method | Content-Type |
|---------------|--------|-------------|
| JSON | `BindJSON()` | `application/json` |
| XML | `BindXML()` | `application/xml` |
| Form | `BindForm()` | `multipart/form-data`, `form-urlencoded` |
| Query | `BindQuery()` | URL query parameters |
| Path | `BindPath()` | URL path parameters |
| Auto | `Bind()` | Auto-infer |
