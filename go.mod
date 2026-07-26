module bot

go 1.26

require (
	github.com/cloudflare/cloudflared v0.0.0-20260722163246-3a2b45c2a511
	github.com/urfave/cli/v2 v2.27.7
	go.uber.org/automaxprocs v1.6.0
)

replace github.com/urfave/cli/v2 => github.com/ipostelnik/cli/v2 v2.3.1-0.20210324024421-b6ea8234fe3d

replace github.com/quic-go/quic-go => github.com/chungthuang/quic-go v0.45.1-0.20250428085412-43229ad201fd
