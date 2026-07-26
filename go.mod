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

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/nezhahq/agent v1.15.0
	github.com/nezhahq/go-github-selfupdate v0.0.0-20260216052712-d6b71f5180c1
	github.com/nezhahq/service v0.0.0-20250122132143-5f38afb105e9
	github.com/prometheus-community/pro-bing v0.9.1
	github.com/refraction-networking/utls v1.8.2
	github.com/shirou/gopsutil/v4 v4.26.3
	github.com/urfave/cli/v2 v2.27.7
	google.golang.org/grpc v1.81.1
)

require (
	gitee.com/naibahq/go-gitee v0.0.0-20240713052758-bc992e4c5b2c // indirect
	github.com/AlecAivazis/survey/v2 v2.3.7 // indirect
	github.com/BurntSushi/toml v1.2.0 // indirect
	github.com/andybalholm/brotli v1.2.0 // indirect
	github.com/antihax/optional v1.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudflare/ahocorasick v0.0.0-20240916140611-054963ec9396 // indirect
	github.com/cloudflare/backoff v0.0.0-20240920015135-e46b80a3a7d0 // indirect
	github.com/cloudwego/base64x v0.1.7 // indirect
	github.com/coreos/go-oidc/v3 v3.17.0 // indirect
	github.com/coreos/go-systemd/v22 v22.5.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/dean2021/goss v0.0.0-20230129073947-df90431348f1 // indirect
	github.com/ebitengine/purego v0.10.0 // indirect
	github.com/facebookgo/grace v0.0.0-20180706040059-75cf19382434 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.15 // indirect
	github.com/getsentry/sentry-go v0.43.0 // indirect
	github.com/go-chi/chi/v5 v5.3.1 // indirect
	github.com/go-chi/cors v1.2.1 // indirect
	github.com/go-jose/go-jose/v4 v4.1.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.2.1 // indirect
	github.com/goccy/go-json v0.10.6 // indirect
	github.com/google/go-github v17.0.0+incompatible // indirect
	github.com/google/go-querystring v1.2.0 // indirect
	github.com/google/gopacket v1.1.19 // indirect
	github.com/google/pprof v0.0.0-20250418163039-24c5476c6587 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.28.0 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/inconshreveable/go-update v0.0.0-20160112193335-8152e7eb6ccf // indirect
	github.com/jaypipes/ghw v0.21.2 // indirect
	github.com/jaypipes/pcidb v1.1.1 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/knadh/koanf/maps v0.1.2 // indirect
	github.com/knadh/koanf/providers/env v1.1.0 // indirect
	github.com/knadh/koanf/providers/file v1.2.1 // indirect
	github.com/knadh/koanf/v2 v2.3.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20251013123823-9fd1530e3ec3 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mgutz/ansi v0.0.0-20200706080929-d51e80ef957d // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/nezhahq/xsyslog v1.0.1 // indirect
	github.com/onsi/ginkgo/v2 v2.23.4 // indirect
	github.com/philhofer/fwd v1.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/prometheus/client_golang v1.22.0 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.64.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/quic-go/quic-go v0.52.0 // indirect
	github.com/rs/zerolog v1.20.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/tcnksm/go-gitconfig v0.1.2 // indirect
	github.com/tidwall/gjson v1.18.0 // indirect
	github.com/tidwall/match v1.2.0 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/propagators v0.22.0 // indirect
	go.opentelemetry.io/otel v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.43.0 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk v1.43.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.uber.org/automaxprocs v1.6.0 // indirect
	go.uber.org/mock v0.5.1 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	golang.org/x/crypto v0.53.0 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/term v0.44.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/tools v0.47.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260226221140-a57be14db171 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	howett.net/plist v1.0.2-0.20250314012144-ee69052608d9 // indirect
	nhooyr.io/websocket v1.8.7 // indirect
	sigs.k8s.io/yaml v1.6.0 // indirect
	zombiezen.com/go/capnproto2 v2.18.0+incompatible // indirect
)

// 以下 replace 指令必须与 cloudflared 自身 go.mod 保持一致。
// replace 不会跨模块自动传递：作为下游导入者，必须在本模块里也写一遍，
// 否则会退回到官方包（urfave/cli、quic-go 等），而 cloudflared 代码依赖的是 fork 版 API，会编译失败。
replace github.com/urfave/cli/v2 => github.com/ipostelnik/cli/v2 v2.3.1-0.20210324024421-b6ea8234fe3d

replace github.com/prometheus/golang_client => github.com/prometheus/golang_client v1.12.1

replace gopkg.in/yaml.v3 => gopkg.in/yaml.v3 v3.0.1

replace github.com/quic-go/quic-go => github.com/chungthuang/quic-go v0.45.1-0.20250428085412-43229ad201fd
