package powerdns

// Zone represents a PowerDNS zone for API requests/responses
// See: https://doc.powerdns.com/authoritative/http-api/zone.html
type Zone struct {
	// ID is opaque zone id assigned by the server (read-only)
	ID string `json:"id,omitempty"`
	// Name of the zone (e.g. "example.com.") MUST have a trailing dot
	Name string `json:"name"`
	// Type is always "Zone" (read-only)
	Type string `json:"type,omitempty"`
	// Kind is zone kind: "Native", "Master", "Slave", "Producer", "Consumer"
	Kind string `json:"kind,omitempty"`
	// Masters is list of IP addresses configured as primary for this zone (Slave zones only)
	Masters []string `json:"masters,omitempty"`
	// Nameservers is list of nameservers for zone creation
	Nameservers []string `json:"nameservers,omitempty"`
	// Account is the zone account for ownership tracking
	Account string `json:"account,omitempty"`
	// RRsets are the resource record sets in this zone
	RRsets []RRset `json:"rrsets,omitempty"`
}

// RRset represents a Resource Record Set (all records with the same name and type)
// See: https://doc.powerdns.com/authoritative/http-api/zone.html
type RRset struct {
	// Name for record set (e.g. "www.powerdns.com.")
	Name string `json:"name"`
	// Type of this record (e.g. "A", "PTR", "MX")
	Type string `json:"type"`
	// TTL is the DNS TTL of the records in seconds.
	// MUST NOT be included when changetype is "DELETE"
	TTL uint32 `json:"ttl,omitempty"`
	// ChangeType MUST be added when updating the RRSet.
	// One of: DELETE, EXTEND, PRUNE, REPLACE
	// EXTEND and PRUNE available since 4.9.12 and 5.0.2
	ChangeType string `json:"changetype,omitempty"`
	// Records in this RRset
	Records []Record `json:"records,omitempty"`
	// Comments on this RRset
	Comments []Comment `json:"comments,omitempty"`
}

// Record represents a single DNS record
// See: https://doc.powerdns.com/authoritative/http-api/zone.html
type Record struct {
	// Content is the content of this record
	Content string `json:"content"`
	// Disabled indicates whether this record is disabled
	Disabled bool `json:"disabled"`
	// ModifiedAt is timestamp of last change (read-only, not sent to server)
}

// Comment represents a comment on an RRSet
// See: https://doc.powerdns.com/authoritative/http-api/zone.html
type Comment struct {
	// Content is the actual comment text
	Content string `json:"content"`
	// Account is name of the account that added the comment
	Account string `json:"account"`
	// ModifiedAt is timestamp of last change (read-only, not sent to server)
}

// ZonePatch represents a PATCH request body for modifying zone RRsets
type ZonePatch struct {
	RRsets []RRset `json:"rrsets"`
}

// APIError represents an error response from PowerDNS API
type APIError struct {
	Error string `json:"error"`
}
