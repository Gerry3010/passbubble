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

package apiclient

// VerifyTOTP completes the second step of a 2FA login and returns the full
// session (tokens + encrypted key material).
func (c *Client) VerifyTOTP(pendingToken, code string) (*LoginResponse, error) {
	var resp LoginResponse
	return &resp, c.post("/api/v1/auth/verify-totp",
		VerifyTOTPRequest{PendingToken: pendingToken, Code: code}, &resp)
}

// SetupTOTP starts account-2FA enrollment and returns a fresh secret + otpauth URL.
// Nothing is enabled until ConfirmTOTP succeeds. Requires an authenticated client.
func (c *Client) SetupTOTP() (*SetupTOTPResponse, error) {
	var resp SetupTOTPResponse
	return &resp, c.post("/api/v1/auth/totp/setup", nil, &resp)
}

// ConfirmTOTP verifies a code against the pending secret and enables 2FA.
func (c *Client) ConfirmTOTP(secret, code string) error {
	return c.post("/api/v1/auth/totp/confirm", ConfirmTOTPRequest{Secret: secret, Code: code}, nil)
}

// DisableTOTP turns off account-2FA. Provide a current code or the account password.
func (c *Client) DisableTOTP(code, password string) error {
	return c.post("/api/v1/auth/totp/disable", DisableTOTPRequest{Code: code, Password: password}, nil)
}

// RequestTOTPRecovery emails a one-time link that disables 2FA. Callable only
// with a valid pending token (i.e. after the password step).
func (c *Client) RequestTOTPRecovery(pendingToken string) error {
	return c.post("/api/v1/auth/totp/recover", RequestTOTPRecoveryRequest{PendingToken: pendingToken}, nil)
}
