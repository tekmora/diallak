package server

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"strings"

	"github.com/tekmora/diallak/internal/roleconn"
)

//go:embed role-connections.json
var roleConnectionsJSON string

//go:embed role-connection-values.json
var roleConnectionValuesJSON string

// RoleConnections loads and validates the embedded role-connection metadata records and values.
// It returns an error naming the count and each finding when validation returns any.
func RoleConnections() (*roleconn.Records, *roleconn.Values, error) {
	recs, err := roleconn.LoadRecords(strings.NewReader(roleConnectionsJSON))
	if err != nil {
		return nil, nil, fmt.Errorf("load embedded role connections: %w", err)
	}
	vals, err := roleconn.LoadValues(strings.NewReader(roleConnectionValuesJSON))
	if err != nil {
		return nil, nil, fmt.Errorf("load embedded role connection values: %w", err)
	}
	findings := roleconn.Validate(recs, vals)
	if len(findings) > 0 {
		return nil, nil, fmt.Errorf("validate role connections: %d findings: %s", len(findings), strings.Join(findings, "; "))
	}
	return recs, vals, nil
}

// RoleConnectionsDigest returns the sha256 checksum of the embedded role-connections.json as "sha256:<hex>".
func RoleConnectionsDigest() string {
	sum := sha256.Sum256([]byte(roleConnectionsJSON))
	return fmt.Sprintf("sha256:%x", sum)
}
