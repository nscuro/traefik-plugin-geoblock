# 🛡️ traefik-plugin-geoblock

[![Build Status](https://github.com/nscuro/traefik-plugin-geoblock/actions/workflows/ci.yml/badge.svg)](https://github.com/nscuro/traefik-plugin-geoblock/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/nscuro/traefik-plugin-geoblock)](https://goreportcard.com/report/github.com/nscuro/traefik-plugin-geoblock)
[![Latest GitHub release](https://img.shields.io/github/v/release/nscuro/traefik-plugin-geoblock?sort=semver)](https://github.com/nscuro/traefik-plugin-geoblock/releases/latest)
[![License](https://img.shields.io/badge/license-Apache%202.0-brightgreen.svg)](LICENSE)  

A Traefik plugin that allows or blocks requests based on IP geolocation using IP2Location database.

> 🌍 This project includes IP2Location LITE data available from [`lite.ip2location.com`](https://lite.ip2location.com/database/ip-country).

## ✨ Features

- Block or allow requests based on country of origin (using ISO 3166-1 alpha-2 country codes)
- Whitelist specific IP ranges (CIDR notation)
- Blacklist specific IP ranges (CIDR notation)
- Optional bypass using custom headers
- Configurable handling of private/internal networks
- Customizable error responses
- Flexible logging options
- Automatic database updates

## 📥 Installation

It is possible to install the [plugin locally](https://traefik.io/blog/using-private-plugins-in-traefik-proxy-2-5/) or to install it through [Traefik Plugins]([Plugins](https://plugins.traefik.io/plugins)).

### Local Plugin Installation

Create or modify your Traefik static configuration

```yaml
experimental:
  localPlugins:
    geoblock:
      moduleName: github.com/nscuro/traefik-plugin-geoblock
```

You should clone the plugin into the container, i.e

```dockerfile
# Create the directory for the plugins
RUN set -eux; \
    mkdir -p /plugins-local/src/github.com/nscuro

RUN set -eux && git clone https://github.com/david-garcia-garcia/traefik-plugin-geoblock /plugins-local/src/github.com/nscuro/traefik-plugin-geoblock --branch v1.0.0-beta.5 --single-branch
```

### Traefik Plugin Registry Installation

Add to your Traefik static configuration:

```yaml
experimental:
  plugins:
    geoblock:
      moduleName: github.com/nscuro/traefik-plugin-geoblock
      version: v0.5.0
```

## ⚙️ Configuration

### Example Docker Compose Setup

```yaml
version: "3.7"

services:
  traefik:
    image: traefik:latest
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./traefik.yml:/etc/traefik/traefik.yml
      - ./dynamic-config.yml:/etc/traefik/dynamic-config.yml
      - ./IP2LOCATION-LITE-DB1.IPV6.BIN:/plugins-storage/IP2LOCATION-LITE-DB1.IPV6.BIN
    ports:
      - "80:80"
      - "443:443"

  whoami:
    image: traefik/whoami
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.whoami.rule=Host(`whoami.localhost`)"
      - "traefik.http.routers.whoami.middlewares=geoblock@file"
```

### Dynamic Configuration

```yaml
http:
  middlewares:
    geoblock:
      plugin:
        geoblock:
          #-------------------------------
          # Core Settings
          #-------------------------------
          enabled: true                   # Enable/disable the plugin entirely
          defaultAllow: false             # Default behavior when no rules match (false = block)
          
          #-------------------------------
          # Database Configuration
          #-------------------------------
          databaseFilePath: "/plugins-local/src/github.com/nscuro/traefik-plugin-geoblock/IP2LOCATION-LITE-DB1.IPV6.BIN"
          # Can be:
          # - Full path: /path/to/IP2LOCATION-LITE-DB1.IPV6.BIN
          # - Directory: /path/to/ (will search for IP2LOCATION-LITE-DB1.IPV6.BIN recursively). Use /plugins-storage/sources/ if you are installing from plugin repository.
          # - Empty: uses embedded database assuming it is installed in /plugins-local/src/github.com/nscuro/traefik-plugin-geoblock/
          
          #-------------------------------
          # Country-based Rules (ISO 3166-1 alpha-2 format)
          #-------------------------------
          allowedCountries:               # Whitelist of countries to allow
            - "US"                        # United States
            - "CA"                        # Canada
            - "GB"                        # United Kingdom
          blockedCountries:               # Blacklist of countries to block
            - "RU"                        # Russia
            - "CN"                        # China
            
          #-------------------------------
          # Network Rules
          #-------------------------------
          allowPrivate: true              # Allow requests from private/internal networks (marked as "PRIVATE")
          allowedIPBlocks:                # CIDR ranges to always allow (highest priority)
            - "192.168.0.0/16"
            - "10.0.0.0/8"
            - "2001:db8::/32"
          blockedIPBlocks:                 # CIDR ranges to always block
            - "203.0.113.0/24"
            # More specific ranges (longer prefix) take precedence
          
          #-------------------------------
          # Bypass Configuration
          #-------------------------------
          bypassHeaders:                  # Headers that skip geoblocking entirely
            X-Internal-Request: "true"
            X-Skip-Geoblock: "1"
            
          #-------------------------------
          # Error Handling and ban
          #-------------------------------
          banIfError: true                # Block requests if IP lookup fails
          disallowedStatusCode: 403       # HTTP status code for blocked requests. If you are using banHtmlFilePath make sure to set this to a valid code (such as NOT 204).
          
          banHtmlFilePath: "/plugins-local/src/github.com/nscuro/traefik-plugin-geoblock/geoblockban.html"
          # Can be:
          # - Full path: /path/to/geoblockban.html
          # - Directory: /path/to/ (will search for geoblockban.html recursively). Use /plugins-storage/sources/ if you are installing from plugin repository.
          # - Empty: returns only status code
          # Template variables available: {{.IP}} and {{.Country}}
          
          #-------------------------------
          # Logging Configuration
          #-------------------------------
          logLevel: "info"                  # Available: debug, info, warn, error
          logFormat: "json"                 # Available: json, text
          logPath: "/var/log/geoblock.log"  # Empty for Traefik's standard output
          logBannedRequests: true           # Log blocked requests. They will be logged at info level.

          #-------------------------------
          # Database Auto-Update Settings
          #-------------------------------
          databaseAutoUpdate: true                   
          # Enable automatic database updates. Updates are asynchronous and triggere during middleware startup. The updated database will be used when the middleware starts again.
          # Make sure you whitelist in your FW domains ["download.ip2location.com", "www.ip2location.com"]
          databaseAutoUpdateDir: "/data/ip2database" 
          # Directory to store updated databases. This must be a persitent volme in the traefik pod.
          databaseAutoUpdateToken: ""                # IP2Location download token (if using premium)
          databaseAutoUpdateCode: "DB1"              # Database product code to download (if using premium)

```

### 🔄 Processing Order

The plugin processes requests in the following order:

1. Check if plugin is enabled
2. Check bypass headers
3. Extract IP addresses from X-Forwarded-For and X-Real-IP headers
4. For each IP:
   - Check if it's in private network range [allowPrivate]
   - Check allowed/blocked IP blocks [allowedIPBlocks, blockedIPBlocks] (most specific match wins)
   - Look up country code 
   - Check allowed/blocked countries [allowedCountries, blockedCountries]
   - Apply default allow/deny if no rules match [defaultAllow]

If any IP in the chain is blocked, the request is denied.

---

📄 This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
