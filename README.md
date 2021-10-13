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
      version: v0.1.1
```

### Dynamic

```yaml
http:
  middlewares:
    geoblock:
      plugin:
        geoblock:
          # Whether or not to enable geoblocking.
          enabled: true
          # Path to the ip2location database.
          databaseFilePath: /plugins-local/src/github.com/nscuro/traefik-plugin-geoblock/IP2LOCATION-LITE-DB1.IPV6.BIN
          # Countries to allow requests from, using ISO 3166-1 alpha-2 codes.
          # See https://en.wikipedia.org/wiki/ISO_3166-1_alpha-2#Officially_assigned_code_elements
          allowedCountries: [ "AT", "CH", "DE" ]
          # Whether or not requests from private networks should be allowed.
          allowPrivate: true
```