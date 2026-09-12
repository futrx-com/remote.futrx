package constants

import "time"

// SMTPDialTimeout bounds how long the SMTP integration waits to connect,
// upgrade TLS, authenticate, and complete one verify or send operation
// before giving up.
const SMTPDialTimeout = 15 * time.Second
