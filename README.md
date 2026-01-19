# PowerDNS Zone Manager

CLI tool for managing PowerDNS zones and records. Creates zones, manages RRsets, tracks ownership via comments.

Only touches records it owns (identified by `account` field in comments). Everything else is left alone.

## Installation

```bash
go install github.com/kreigan/powerdns-zone-manager@latest
```

Or grab a binary from [releases](https://github.com/kreigan/powerdns-zone-manager/releases).

## Usage

```bash
# Apply configuration
powerdns-zone-manager apply \
  --api-url http://localhost:8081/api/v1/servers/localhost \
  --api-key your-api-key \
  zones.yml

# Dry run (see what would change)
powerdns-zone-manager apply --dry-run ...

# Verbose output
powerdns-zone-manager apply -v ...

# JSON output (for automation)
powerdns-zone-manager apply --json ...
```

Custom account name (default: `zone-manager`):
```bash
ACCOUNT_NAME=my-tool powerdns-zone-manager apply ...
```

## Configuration File

```yaml
zones:
  example.local:
    kind: Native  # optional, defaults to Native
    nameservers:  # required for new zones
      - ns1.example.local.
      - ns2.example.local.

    rrsets:
      - name: ns1
        type: A
        ttl: 86400
        records: 10.1.21.3

      - name: '@'  # zone apex
        type: A
        records:
          - 192.168.1.1
          - 192.168.1.2

      - name: www
        type: CNAME
        records: example.local.

      - name: mail
        type: MX
        records:
          - content: 10 mail.example.local.
          - content: 20 backup.example.local.
            disabled: true
```

## Zones File Syntax

**Zone options:**
- `kind` — Zone type: Native, Master, Slave, Producer, Consumer. Defaults to Native.
- `nameservers` — Required when creating a zone. Controls NS records. Must end with `.` or PowerDNS appends the zone name automatically.

**RRset options:**
- `name` — Record name. Use `@` for zone apex.
- `type` — DNS record type. NS and SOA are not allowed here (use `nameservers` for NS).
- `ttl` — TTL in seconds. Defaults to 300.
- `records` — Single value, list of strings, or list of objects with `content`, `disabled`, `comment`.

**Records format:**
```yaml
# Single value
records: 192.168.1.1

# List of strings
records:
  - 192.168.1.1
  - 192.168.1.2

# Objects with options
records:
  - content: 192.168.1.1
    disabled: true
    comment: Maintenance
```

## License

MIT
