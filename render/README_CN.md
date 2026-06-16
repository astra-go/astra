# Render — 服务端模板渲染

基于 Go 标准库 `html/template` 的服务端模板渲染引擎，支持布局、局部模板、热更新和 `embed.FS`。

## 特性

- **布局系统**：主模板 + 子模板布局嵌套
- **局部模板**：`partials/` 目录下的模板片段可被任意页面引用
- **热更新**：开发模式下文件变化自动重新解析模板
- **embed.FS 支持**：配合 Go 1.16+ `embed` 实现编译时静态资源打包
- **自定义函数**：`FuncMap` 支持注册自定义模板函数

## 快速开始

```go
import (
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/render"
)

// 方式一：文件系统路径
app := astra.New(astra.WithRenderer(render.New(render.Config{
    Root:    "templates",
    Layout:  "layouts/base.html",
    Ext:     ".html",
    Reload:  true, // 开发模式下热更新
})))

app.GET("/page", func(c *astra.Ctx) error {
    return c.Render(200, "pages/index.html", astra.Map{"title": "Home"})
})

// 方式二：embed.FS（生产环境推荐）
//go:embed templates
var tmplFS embed.FS

app := astra.New(astra.WithRenderer(render.New(render.Config{
    FS:     tmplFS,
    Root:   "templates",
    Layout: "layouts/base.html",
})))
```

## 目录结构示例

```
templates/
├── layouts/
│   └── base.html          ← {{block "content" .}} {{end}}
├── partials/
│   ├── header.html        ← {{define "partials/header"}} ... {{end}}
│   └── footer.html
└── pages/
    ├── index.html         ← {{define "title"}}Home{{end}} {{define "content"}} ... {{end}}
    └── user/
        └── profile.html
```

### 在模板中使用局部模板

```html
<!-- pages/index.html -->
{{template "partials/header.html" .}}
<div>{{.title}}</div>
{{template "partials/footer.html" .}}
```

## API

### New

```go
func New(cfg Config) Engine

type Config struct {
    Root    string           // 模板根目录（与 FS 互斥）
    FS      embed.FS         // embed.FS（与 Root 互斥）
    Layout  string           // 主布局模板路径（空=无布局）
    Ext     string           // 文件扩展名，默认 ".html"
    Reload  bool             // 文件变化时重新加载
    FuncMap template.FuncMap // 自定义模板函数
}
```

### c.Render

```go
func (c *astra.Ctx) Render(code int, name string, data any) error
```

`name` 是相对于 Root 的模板路径；`data` 是模板数据。

## 自定义模板函数

```go
app := astra.New(astra.WithRenderer(render.New(render.Config{
    Root:   "templates",
    Layout: "layouts/base.html",
    FuncMap: template.FuncMap{
        "upper": strings.ToUpper,
        "formatDate": func(t time.Time) string {
            return t.Format("2006-01-02")
        },
        "truncate": func(s string, n int) string {
            if len(s) > n {
                return s[:n] + "..."
            }
            return s
        },
    },
})))
```

## 完整示例

```go
package main

import (
    "embed"
    "github.com/astra-go/astra"
    "github.com/astra-go/astra/render"
)

//go:embed templates
var tmplFS embed.FS

//go:embed templates/layouts
var _ embed.FS

func main() {
    app := astra.New(astra.WithRenderer(render.New(render.Config{
        FS:     tmplFS,
        Root:   "templates",
        Layout: "layouts/base.html",
        FuncMap: map[string]any{
            "upper": func(s string) string { return strings.ToUpper(s) },
        },
    })))

    app.GET("/", func(c *astra.Ctx) error {
        return c.Render(200, "pages/index.html", astra.Map{
            "title": "Welcome",
            "items": []string{"A", "B", "C"},
        })
    })

    app.Run(":8080")
}
```

## 模块依赖

无外部依赖（使用 Go 标准库 `html/template`）。

## 注意事项

- `Root` 和 `FS` 互斥；`FS` 适合生产环境，避免运行时文件系统依赖
- `Reload: true` 仅用于开发；生产环境应关闭以避免性能开销
- 模板执行错误返回 HTTP 500；建议在开发时验证模板语法