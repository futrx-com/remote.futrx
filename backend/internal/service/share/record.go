package share

// Record is the repository-facing state of one public preview link. The
// plaintext token is never stored; TokenHash is its SHA-256 digest.
type Record struct {
	ID        ID
	TokenHash string
	Port      int
	Label     string
	CreatedBy string
	CreatedAt int64
	ExpiresAt int64
	RevokedAt int64
}

func (r Record) active(nowMilli int64) bool {
	return r.RevokedAt == 0 && r.ExpiresAt > nowMilli
}
