# PowerDNS Zone Manager

A CLI tool for managing PowerDNS zones and resource record sets with ownership tracking.

## Features

- ✅ Creates absent zones
- ✅ Creates, updates, or deletes **managed** record sets in zones
- ✅ Does not touch unmanaged records
- ✅ Ownership tracking using PowerDNS comment accounts
- ✅ Automatic TXT record quote normalization
- ✅ Dry-run mode for safe testing
- ✅ Zone apex support with `@` notation
- ✅ Validates configuration before making changes (fails early with all errors)
- ✅ Single static binary with zero runtime dependencies
- ✅ PowerDNS 5.0.2 compatible

## How Ownership Works

A record set (RRset) is considered **managed** if it has at least one comment where the `account` property value matches the configured account name.

- Default account name: `zone-manager`
- Configurable via `ACCOUNT_NAME` environment variable

When creating a zone, its `account` property is set to the configured account name, marking it as managed.

Only managed RRsets are modified or deleted. Unmanaged records are left untouched.

## Installation

### Download Pre-built Binaries

Download the latest release for your platform from the [releases page](https://github.com/kreigan/powerdns-zone-manager/releases).

```bash
# Linux amd64
curl -LO https://github.com/kreigan/powerdns-zone-manager/releases/latest/download/powerdns-zone-manager_linux_amd64.tar.gz
tar xzf powerdns-zone-manager_linux_amd64.tar.gz
sudo mv powerdns-zone-manager /usr/local/bin/

# macOS arm64
curl -LO https://github.com/kreigan/powerdns-zone-manager/releases/latest/download/powerdns-zone-manager_darwin_arm64.tar.gz
tar xzf powerdns-zone-manager_darwin_arm64.tar.gz
sudo mv powerdns-zone-manager /usr/local/bin/
```

### Build from Source

```bash
git clone https://github.com/kreigan/powerdns-zone-manager.git
cd powerdns-zone-manager
go build -o powerdns-zone-manager .
```

## Usage

### Configuration File Format

Create a YAML file defining your zones and records:

```yaml
zones:
  example.local:
    # kind is optional, defaults to "Native"
    # Valid values: Native, Master, Slave, Producer, Consumer
    kind: Native
    
    # nameservers is required when creating a new zone
    # For existing managed zones, it controls NS records
    # NS records cannot be specified in rrsets
    nameservers:
      - ns.example.local

    rrsets:
      # Full object format with comment
      - name: ns
        type: A
        ttl: 86400  # TTL is optional, defaults to 300
        records:
          - content: 10.1.21.3
            comment: Record comment

      # Single value format (string)
      - name: '@'
        type: A
        records: 192.168.2.1

      # List of strings format
      - name: www
        type: A
        records:
          - 192.168.2.1

      # Mixed format: strings and objects
      - name: test-auto
        type: TXT
        records:
          - Test TXT record
          - content: Another TXT record
            disabled: true  # disabled defaults to false
```

### Record Format Variations

Records can be specified in multiple ways:

```yaml
# Single string value
records: 192.168.1.1

# List of strings
records:
  - 192.168.1.1
  - 192.168.1.2

# List of objects
records:
  - content: 192.168.1.1
    disabled: false
  - content: 192.168.1.2
    comment: Backup server

# Mixed list
records:
  - 192.168.1.1
  - content: 192.168.1.2
    disabled: true
```

### Zone Configuration Rules

1. **Zone kind** defaults to `Native` if not specified
2. **Nameservers** are required only when creating a new zone
3. **NS records** must be managed via the `nameservers` property, not in `rrsets`
4. **SOA records** are managed by PowerDNS automatically
5. **TTL** defaults to 300 seconds if not specified
6. **Disabled** defaults to `false` if not specified

### Apply Configuration

```bash
powerdns-zone-manager apply \
  --api-url http://localhost:8081/api/v1/servers/localhost \
  --api-key your-api-key \
  zones.yml
```

### Dry Run

Test changes without applying them:

```bash
powerdns-zone-manager apply --dry-run \
  --api-url http://localhost:8081/api/v1/servers/localhost \
  --api-key your-api-key \
  zones.yml
```

### Custom Account Name

Set a custom account name for ownership tracking:

```bash
ACCOUNT_NAME=my-automation powerdns-zone-manager apply \
  --api-url http://localhost:8081/api/v1/servers/localhost \
  --api-key your-api-key \
  zones.yml
```

### Verbose Mode

Enable verbose/debug output for troubleshooting:

```bash
powerdns-zone-manager apply -v \
  --api-url http://localhost:8081/api/v1/servers/localhost \
  --api-key your-api-key \
  zones.yml
```

Note: Sensitive data like API keys are automatically masked in verbose output.

## Validation

The tool validates the entire configuration before making any changes and reports **all** errors at once:

- Zone kind must be valid (Native, Master, Slave, Producer, Consumer)
- Nameservers are required for new zones
- Cannot modify zones that are not managed (different account)
- NS and SOA record types are prohibited in rrsets
- RRset names and types are required
- At least one record is required per RRset
- No duplicate RRsets (same name and type)

## API Compatibility

This tool is designed for **PowerDNS Authoritative Server 5.0.2** and uses the following API endpoints:

- `GET /zones/{zone_id}` - Check if zone exists
- `POST /zones` - Create new zone
- `PATCH /zones/{zone_id}` - Modify RRsets

See the [PowerDNS HTTP API documentation](https://doc.powerdns.com/authoritative/http-api/zone.html) for details.

## License

MIT License
