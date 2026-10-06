module github.com/denisdubovitskiy/neomock/example

go 1.26.0

require github.com/denisdubovitskiy/neomock v0.0.0

require (
	github.com/pelletier/go-toml/v2 v2.4.3 // indirect
	golang.org/x/mod v0.39.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/denisdubovitskiy/neomock => ../

tool github.com/denisdubovitskiy/neomock/cmd/neomock
