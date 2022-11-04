# traefik-plugin-geoblock

[![Build Status](https://github.com/nscuro/traefik-plugin-geoblock/actions/workflows/ci.yml/badge.svg)](https://github.com/nscuro/traefik-plugin-geoblock/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/nscuro/traefik-plugin-geoblock)](https://goreportcard.com/report/github.com/nscuro/traefik-plugin-geoblock)
[![Latest GitHub release](https://img.shields.io/github/v/release/nscuro/traefik-plugin-geoblock?sort=semver)](https://github.com/nscuro/traefik-plugin-geoblock/releases/latest)
[![License](https://img.shields.io/badge/license-Apache%202.0-brightgreen.svg)](LICENSE)  

*traefik-plugin-geoblock is a traefik plugin to whitelist requests based on geolocation*

> This projects includes IP2Location LITE data available from [`lite.ip2location.com`](https://lite.ip2location.com/database/ip-country).

## Configuration

### Static

#### Local

```yaml
experimental:
  localPlugins:
    geoblock:
      moduleName: github.com/nscuro/traefik-plugin-geoblock
```

#### Pilot

```yaml
pilot:
  token: "xxxxxxxxx"

experimental:
  plugins:
    geoblock:
      moduleName: github.com/nscuro/traefik-plugin-geoblock
      version: v0.5.0
```

### Dynamic

```yaml
http:
  middlewares:
    geoblock:
      plugin:
        geoblock:
          # Enable this plugin?
          enabled: true
          # Path to ip2location database file
          databaseFilePath: /plugins-local/src/github.com/nscuro/traefik-plugin-geoblock/IP2LOCATION-LITE-DB1.IPV6.BIN
          # Whitelist of countries to allow (ISO 3166-1 alpha-2)
          allowedCountries: [ "AT", "CH", "DE" ]
          # Allow requests from private / internal networks?
          allowPrivate: true
          # HTTP status code to return for disallowed requests (default: 403)
          disallowedStatusCode: 204
          # Add CIDR to be whitelisted, even if in a non-allowed country
          allowedIPBlocks: ["66.249.64.0/19"]
```