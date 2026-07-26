module bot

// cloudflared 当前要求 Go 1.26（CI 需用 go-version: '1.26'）
go 1.26

// 只需声明对 cloudflared 的直接依赖；其余传递依赖由 `go mod tidy` 补全并写入 go.sum。
require github.com/cloudflare/cloudflared v2026.7.3

// 以下 replace 指令必须与 cloudflared 自身 go.mod 保持一致。
// replace 不会跨模块自动传递：作为下游导入者，必须在本模块里也写一遍，
// 否则会退回到官方包（urfave/cli、quic-go 等），而 cloudflared 代码依赖的是 fork 版 API，会编译失败。
replace github.com/urfave/cli/v2 => github.com/ipostelnik/cli/v2 v2.3.1-0.20210324024421-b6ea8234fe3d

replace github.com/prometheus/golang_client => github.com/prometheus/golang_client v1.12.1

replace gopkg.in/yaml.v3 => gopkg.in/yaml.v3 v3.0.1

replace github.com/quic-go/quic-go => github.com/chungthuang/quic-go v0.45.1-0.20250428085412-43229ad201fd
