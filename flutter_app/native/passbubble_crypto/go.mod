module github.com/Gerry3010/passbubble/flutter_native

go 1.26.3

require github.com/Gerry3010/passbubble/backend v0.0.0

require (
	github.com/cloudflare/circl v1.6.3 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
)

replace github.com/Gerry3010/passbubble/backend => ../../../backend
