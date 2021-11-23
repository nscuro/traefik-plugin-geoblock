module github.com/nscuro/traefik-plugin-geoblock

go 1.17

require (
	github.com/ip2location/ip2location-go/v9 v9.1.0
	github.com/stretchr/testify v1.7.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

replace github.com/ip2location/ip2location-go/v9 => github.com/nscuro/ip2location-go/v9 v9.2.0
