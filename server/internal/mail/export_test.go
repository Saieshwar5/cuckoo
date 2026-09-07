package mail

// Helpers a test needs to point a mailer at a server it started itself. They
// live here rather than in the mailer so nothing outside a test can reach
// them: this file is only compiled for tests.

// SetAddrForTesting sends to an address the operating system chose, rather than
// the configured host and port.
func SetAddrForTesting(s *SMTP, addr string) { s.addr = addr }

// TrustAnythingForTesting accepts a certificate nobody signed, which is what a
// test's own server presents.
func TrustAnythingForTesting(s *SMTP) {
	s.tlsCfg.InsecureSkipVerify = true //nolint:gosec // a test's certificate is signed by nobody
}
