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
	"bytes"
	"crypto/tls"
	_ "embed"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/smtp"
	"net/textproto"
	"net/url"
)

// passbubbleIcon is the transparent brand icon (water bubble + "> _" prompt),
// embedded so transactional emails can show it inline via a CID reference —
// no remote image load (which many clients block by default).
//
//go:embed passbubble-icon.png
var passbubbleIcon []byte

// iconCID is the Content-ID the HTML references as `cid:<iconCID>`.
const iconCID = "passbubble-icon"

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
	body := m.render(emailContent{
		command:    "auth verify-email --activate",
		heading:    "Verify your account",
		intro:      "Run the command below to activate your account. The link expires in 24 hours.",
		accent:     colorGreen,
		buttonText: "Verify email",
		link:       link,
		footer:     "If you did not create a Passbubble account, you can safely ignore this email.",
	})
	return m.send(toEmail, "Verify your Passbubble account", body)
}

// SendInvitationEmail sends an invitation with a link to the web app's
// registration screen, pre-filled with the invitation token and email.
func (m *Mailer) SendInvitationEmail(toEmail, token string) error {
	link := fmt.Sprintf("%s/web/#/register?token=%s&email=%s",
		m.baseURL, url.QueryEscape(token), url.QueryEscape(toEmail))
	body := m.render(emailContent{
		command:    "account create --invite",
		heading:    "You've been invited",
		intro:      "You've been invited to create a Passbubble account. Run the command below to set up your vault. The invitation expires in 7 days.",
		accent:     colorGreen,
		buttonText: "Accept invitation",
		link:       link,
		footer:     "If you did not expect this invitation, you can safely ignore this email.",
	})
	return m.send(toEmail, "You've been invited to Passbubble", body)
}

// SendTOTPRecoveryEmail sends a one-time link that disables two-factor
// authentication for the account (used when the authenticator is lost).
func (m *Mailer) SendTOTPRecoveryEmail(toEmail, token string) error {
	link := fmt.Sprintf("%s/api/v1/auth/reset-totp?token=%s", m.baseURL, token)
	body := m.render(emailContent{
		command:    "auth reset-totp --disable",
		heading:    "Reset two-factor auth",
		intro:      "You asked to disable 2FA for your Passbubble account because you lost access to your authenticator. Run the command below — the link expires in 30 minutes.",
		accent:     colorRed,
		buttonText: "Disable 2FA",
		link:       link,
		footer:     "If you did not request this, ignore this email and your 2FA stays enabled.",
	})
	return m.send(toEmail, "Reset Passbubble two-factor authentication", body)
}

// Passbubble terminal design tokens (mirrors flutter_app AppTheme).
const (
	colorBg       = "#212121"
	colorSurface  = "#2A2A2A"
	colorElevated = "#303030"
	colorGreen    = "#00E676"
	colorGreenDim = "#00C853"
	colorOnBg     = "#E0E0E0"
	colorOnBgDim  = "#9E9E9E"
	colorRed      = "#CF6679"
	colorBorder   = "#424242"
	colorBtnText  = "#121212"
	fontMono      = "'JetBrains Mono','SF Mono',SFMono-Regular,Menlo,Consolas,'Liberation Mono',monospace"
)

// emailContent is the per-email payload rendered into the shared terminal frame.
type emailContent struct {
	command    string // shown after the shell prompt, e.g. "auth verify-email"
	heading    string
	intro      string
	accent     string // button / prompt accent colour
	buttonText string
	link       string
	footer     string
}

// render wraps content in the app's terminal-styled HTML frame: a dark
// monospace "window" with a title bar, the Passbubble icon, a shell-prompt
// command line and a square accent button. Built with tables + inline styles
// for broad email-client compatibility.
func (m *Mailer) render(c emailContent) string {
	// Referenced inline via the multipart/related CID attachment (see send).
	iconURL := "cid:" + iconCID

	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"></head>
<body style="margin:0;padding:0;background:%[1]s;-webkit-text-size-adjust:100%%;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:%[1]s;padding:32px 12px;">
    <tr><td align="center">
      <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="max-width:560px;width:100%%;background:%[2]s;border:1px solid %[8]s;">
        <tr><td style="background:%[3]s;border-bottom:1px solid %[8]s;padding:10px 16px;font-family:%[11]s;font-size:12px;color:%[7]s;">
          <span style="color:%[9]s;">&#9679;</span> <span style="color:#E6B800;">&#9679;</span> <span style="color:%[4]s;">&#9679;</span>
          &nbsp;&nbsp;passbubble &mdash; ~/account
        </td></tr>
        <tr><td style="padding:28px 28px 4px;">
          <table role="presentation" cellpadding="0" cellspacing="0"><tr>
            <td style="padding-right:12px;vertical-align:middle;">
              <img src="%[12]s" width="40" height="40" alt="passbubble" style="display:block;border:1px solid %[8]s;">
            </td>
            <td style="vertical-align:middle;font-family:%[11]s;font-size:20px;font-weight:700;color:%[4]s;letter-spacing:0.5px;">passbubble</td>
          </tr></table>
        </td></tr>
        <tr><td style="padding:18px 28px 0;font-family:%[11]s;font-size:13px;color:%[4]s;">
          <span style="color:%[7]s;">user@passbubble</span><span style="color:%[6]s;">:~$</span> %[13]s
        </td></tr>
        <tr><td style="padding:14px 28px 0;font-family:%[11]s;">
          <div style="font-size:17px;font-weight:700;color:%[5]s;margin-bottom:10px;">%[14]s</div>
          <div style="font-size:14px;line-height:1.6;color:%[5]s;">%[15]s</div>
        </td></tr>
        <tr><td style="padding:24px 28px 4px;">
          <a href="%[16]s" style="display:inline-block;background:%[10]s;color:%[18]s;font-family:%[11]s;font-weight:700;font-size:14px;text-transform:uppercase;letter-spacing:0.5px;padding:13px 24px;text-decoration:none;border:1px solid %[10]s;">&#9656; %[17]s</a>
        </td></tr>
        <tr><td style="padding:16px 28px 24px;font-family:%[11]s;font-size:12px;color:%[7]s;line-height:1.6;">
          <span style="color:%[7]s;"># or paste this URL into your browser:</span><br>
          <a href="%[16]s" style="color:%[6]s;word-break:break-all;">%[16]s</a>
        </td></tr>
        <tr><td style="padding:14px 28px;border-top:1px solid %[8]s;font-family:%[11]s;font-size:11px;color:%[7]s;line-height:1.6;">
          %[19]s
        </td></tr>
      </table>
    </td></tr>
  </table>
</body>
</html>`,
		colorBg,       // 1
		colorSurface,  // 2
		colorElevated, // 3
		colorGreen,    // 4
		colorOnBg,     // 5
		colorGreenDim, // 6
		colorOnBgDim,  // 7
		colorBorder,   // 8
		colorRed,      // 9
		c.accent,      // 10
		fontMono,      // 11
		iconURL,       // 12
		c.command,     // 13
		c.heading,     // 14
		c.intro,       // 15
		c.link,        // 16
		c.buttonText,  // 17
		colorBtnText,  // 18
		c.footer,      // 19
	)
}

// send delivers an HTML email (with the inline brand icon) to a single
// recipient. Port 465 uses implicit TLS (SMTPS); all other ports use STARTTLS
// via smtp.SendMail.
func (m *Mailer) send(toEmail, subject, htmlBody string) error {
	msg, err := m.buildMessage(toEmail, subject, htmlBody)
	if err != nil {
		return fmt.Errorf("build message: %w", err)
	}

	addr := net.JoinHostPort(m.host, m.port)

	if m.port == "465" {
		return m.sendImplicitTLS(addr, toEmail, msg)
	}

	var auth smtp.Auth
	if m.user != "" {
		auth = smtp.PlainAuth("", m.user, m.password, m.host)
	}
	return smtp.SendMail(addr, auth, m.from, []string{toEmail}, msg)
}

// buildMessage assembles a multipart/related MIME message: the HTML body plus
// the brand icon as an inline part the HTML references via `cid:passbubble-icon`.
func (m *Mailer) buildMessage(toEmail, subject, htmlBody string) ([]byte, error) {
	var related bytes.Buffer
	mw := multipart.NewWriter(&related)

	htmlPart, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"text/html; charset=UTF-8"},
		"Content-Transfer-Encoding": {"base64"},
	})
	if err != nil {
		return nil, err
	}
	writeBase64Lines(htmlPart, []byte(htmlBody))

	imgPart, err := mw.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"image/png"},
		"Content-Transfer-Encoding": {"base64"},
		"Content-ID":                {"<" + iconCID + ">"},
		"Content-Disposition":       {`inline; filename="passbubble-icon.png"`},
	})
	if err != nil {
		return nil, err
	}
	writeBase64Lines(imgPart, passbubbleIcon)

	if err := mw.Close(); err != nil {
		return nil, err
	}

	var msg bytes.Buffer
	fmt.Fprintf(&msg, "From: Passbubble <%s>\r\n", m.from)
	fmt.Fprintf(&msg, "To: %s\r\n", toEmail)
	fmt.Fprintf(&msg, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&msg, "Content-Type: multipart/related; type=\"text/html\"; boundary=%q\r\n\r\n", mw.Boundary())
	msg.Write(related.Bytes())
	return msg.Bytes(), nil
}

// writeBase64Lines writes data as base64 wrapped at 76 columns with CRLF line
// endings, as required for SMTP message bodies.
func writeBase64Lines(w io.Writer, data []byte) {
	enc := base64.StdEncoding.EncodeToString(data)
	for len(enc) > 76 {
		_, _ = io.WriteString(w, enc[:76]+"\r\n")
		enc = enc[76:]
	}
	_, _ = io.WriteString(w, enc+"\r\n")
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
