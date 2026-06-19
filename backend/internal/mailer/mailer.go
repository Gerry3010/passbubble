// Copyright (C) 2026 Gerald Hofbauer <info@geraldhofbauer.net>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package mailer

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
)

// Mailer sends transactional emails via SMTP.
// Port 587 uses STARTTLS; port 465 uses implicit TLS (SMTPS).
type Mailer struct {
	host     string
	port     string
	user     string
	password string
	from     string
	baseURL  string
}

func New(host, port, user, password, from, baseURL string) *Mailer {
	return &Mailer{
		host:     host,
		port:     port,
		user:     user,
		password: password,
		from:     from,
		baseURL:  baseURL,
	}
}

// SendVerificationEmail sends a one-time email-verification link to toEmail.
func (m *Mailer) SendVerificationEmail(toEmail, token string) error {
	link := fmt.Sprintf("%s/api/v1/auth/verify-email?token=%s", m.baseURL, token)

	body := fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family:sans-serif;max-width:560px;margin:40px auto;color:#222">
  <h2>Verify your Passbubble account</h2>
  <p>Click the button below to activate your account. The link expires in 24 hours.</p>
  <p style="margin:32px 0">
    <a href="%s"
       style="background:#2563eb;color:#fff;padding:12px 24px;border-radius:6px;text-decoration:none;font-weight:600">
      Verify email address
    </a>
  </p>
  <p style="font-size:13px;color:#666">
    Or copy this link into your browser:<br>
    <a href="%s">%s</a>
  </p>
  <p style="font-size:12px;color:#999;margin-top:40px">
    If you did not create a Passbubble account, you can ignore this email.
  </p>
</body>
</html>`, link, link, link)

	return m.send(toEmail, "Verify your Passbubble account", body)
}

// SendTOTPRecoveryEmail sends a one-time link that disables two-factor
// authentication for the account (used when the authenticator is lost).
func (m *Mailer) SendTOTPRecoveryEmail(toEmail, token string) error {
	link := fmt.Sprintf("%s/api/v1/auth/reset-totp?token=%s", m.baseURL, token)

	body := fmt.Sprintf(`<!DOCTYPE html>
<html>
<body style="font-family:sans-serif;max-width:560px;margin:40px auto;color:#222">
  <h2>Reset two-factor authentication</h2>
  <p>You asked to disable 2FA for your Passbubble account because you lost access to
     your authenticator. Click the button below — the link expires in 30 minutes.</p>
  <p style="margin:32px 0">
    <a href="%s"
       style="background:#dc2626;color:#fff;padding:12px 24px;border-radius:6px;text-decoration:none;font-weight:600">
      Disable 2FA
    </a>
  </p>
  <p style="font-size:13px;color:#666">
    Or copy this link into your browser:<br>
    <a href="%s">%s</a>
  </p>
  <p style="font-size:12px;color:#999;margin-top:40px">
    If you did not request this, ignore this email and your 2FA stays enabled.
  </p>
</body>
</html>`, link, link, link)

	return m.send(toEmail, "Reset Passbubble two-factor authentication", body)
}

// send delivers an HTML email to a single recipient. Port 465 uses implicit
// TLS (SMTPS); all other ports use STARTTLS via smtp.SendMail.
func (m *Mailer) send(toEmail, subject, htmlBody string) error {
	msg := fmt.Sprintf(
		"From: Passbubble <%s>\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		m.from, toEmail, subject, htmlBody,
	)

	addr := net.JoinHostPort(m.host, m.port)

	if m.port == "465" {
		return m.sendImplicitTLS(addr, toEmail, []byte(msg))
	}

	var auth smtp.Auth
	if m.user != "" {
		auth = smtp.PlainAuth("", m.user, m.password, m.host)
	}
	return smtp.SendMail(addr, auth, m.from, []string{toEmail}, []byte(msg))
}

// sendImplicitTLS dials with TLS first (port 465 / SMTPS), then authenticates.
func (m *Mailer) sendImplicitTLS(addr, to string, msg []byte) error {
	tlsCfg := &tls.Config{ServerName: m.host}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("tls dial: %w", err)
	}
	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = client.Close() }()

	if m.user != "" {
		auth := smtp.PlainAuth("", m.user, m.password, m.host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}
	if err := client.Mail(m.from); err != nil {
		return fmt.Errorf("smtp mail from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("smtp rcpt: %w", err)
	}
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp data: %w", err)
	}
	if _, err := wc.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	return wc.Close()
}
