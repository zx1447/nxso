module bot

// cloudflared 当前要求 Go 1.26（CI 需用 go-version: '1.26'）
go 1.26

// cloudflared 的 git tag 是 "2026.7.3"（无 v 前缀，且 major=2026>=2）。
// 因为 module path 没有 /v2026 后缀，Go 既不认无 v 前缀的 tag，
// 也不能直接 require v2026.7.3（需 +incompatible 但仍找不到该 tag）。
// 正确做法：用 commit 伪版本（major=0，无需 /vN 也无需 +incompatible）。
// 该伪版本由 commit 3a2b45c2a511fcdd...（tag 2026.7.3 指向的 commit）得到，
// 本地已用 Go 1.26.5 实测可解析：v0.0.0-20260722163246-3a2b45c2a511
require github.com/cloudflare/cloudflared v0.0.0-20260722163246-3a2b45c2a511

// 以下 replace 指令必须与 cloudflared 自身 go.mod 保持一致。
// replace 不会跨模块自动传递：作为下游导入者，必须在本模块里也写一遍，
// 否则会退回到官方包（urfave/cli、quic-go 等），而 cloudflared 代码依赖的是 fork 版 API，会编译失败。
replace github.com/urfave/cli/v2 => github.com/ipostelnik/cli/v2 v2.3.1-0.20210324024421-b6ea8234fe3d

replace github.com/prometheus/golang_client => github.com/prometheus/golang_client v1.12.1

replace gopkg.in/yaml.v3 => gopkg.in/yaml.v3 v3.0.1

replace github.com/quic-go/quic-go => github.com/chungthuang/quic-go v0.45.1-0.20250428085412-43229ad201fd
