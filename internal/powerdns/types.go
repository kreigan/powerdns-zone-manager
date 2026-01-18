package powerdns

// Zone represents a PowerDNS zone for API requests/responses.
// See: https://doc.powerdns.com/authoritative/http-api/zone.html
type Zone struct {
	ID          string   `json:"id,omitempty"`
	Name        string   `json:"name"`
	Type        string   `json:"type,omitempty"`
	Kind        string   `json:"kind,omitempty"`
	Account     string   `json:"account,omitempty"`
	Masters     []string `json:"masters,omitempty"`
	Nameservers []string `json:"nameservers,omitempty"`
	RRsets      []RRset  `json:"rrsets,omitempty"`
}

// RRset represents a Resource Record Set (all records with the same name and type).
// See: https://doc.powerdns.com/authoritative/http-api/zone.html
type RRset struct {
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	ChangeType string    `json:"changetype,omitempty"`
	Records    []Record  `json:"records,omitempty"`
	Comments   []Comment `json:"comments,omitempty"`
	TTL        uint32    `json:"ttl,omitempty"`
}

// Record represents a single DNS record.
// See: https://doc.powerdns.com/authoritative/http-api/zone.html
type Record struct {
	// Content is the content of this record
	Content string `json:"content"`
	// Disabled indicates whether this record is disabled
	Disabled bool `json:"disabled"`
}

// Comment represents a comment on an RRSet.
// See: https://doc.powerdns.com/authoritative/http-api/zone.html
type Comment struct {
	// Content is the actual comment text
	Content string `json:"content"`
	// Account is name of the account that added the comment
	Account string `json:"account"`
}

// ZonePatch represents a PATCH request body for modifying zone RRsets.
type ZonePatch struct {
	RRsets []RRset `json:"rrsets"`
}

// APIError represents an error response from PowerDNS API.
type APIError struct {
	Error string `json:"error"`
}
