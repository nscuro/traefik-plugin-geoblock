# traefik-plugin-geoblock

> This projects includes IP2Location LITE data available from [`lite.ip2location.com`](https://lite.ip2location.com/database/ip-country).

## Configuration

### Static

```yaml
experimental:
  localPlugins:
    geoblock:
      moduleName: github.com/nscuro/traefik-plugin-geoblock
```

### Dynamic

```yaml
http:
  middlewares:
    geoblock:
      plugin:
        geoblock:
          enabled: true
          databaseFilePath: /plugins-local/src/github.com/nscuro/traefik-plugin-geoblock/IP2LOCATION-LITE-DB1.IPV6.BIN
          allowedCountries: [ "AT", "CH", "DE" ]
          allowPrivate: true
```