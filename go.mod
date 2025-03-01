module github.com/nscuro/traefik-plugin-geoblock

go 1.21

replace github.com/ip2location/ip2location-go/v9 v9.7.1 => github.com/david-garcia-garcia/ip2location-go/v9 v9.7.1-safe

require github.com/ip2location/ip2location-go/v9 v9.7.1

require lukechampine.com/uint128 v1.3.0 // indirect
