# TOTP Progress Bar Fix Test

## Issue
The TOTP progress bar in the TUI wasn't automatically counting down. There were two problems:
1. The code was regenerating the entire TOTP code and remaining time on each tick instead of just decrementing the remaining time
2. The timer wasn't being continued after each tick - it was returning `nil` command which stopped the timer

## Fix Applied
Modified the `TickMsg` handler in `/internal/tui/main.go` lines 279-300:

### Before:
```go
case TickMsg:
	if m.screen == DetailScreen && m.detailEntry.Type == "totp" && m.showSecrets {
		return m, m.updateTOTPReal()
	}
	return m, nil
```

### After:
```go
case TickMsg:
	// Schedule next tick to keep timer running
	nextTickCmd := tea.Every(time.Second, func(t time.Time) tea.Msg {
		return TickMsg{Time: t}
	})
	
	if m.screen == DetailScreen && m.detailEntry.Type == "totp" && m.showSecrets {
		// Decrement remaining time if we have an active TOTP code
		if m.totpRemaining > 0 {
			m.totpRemaining--
			// Only regenerate TOTP code when time expires
			if m.totpRemaining <= 0 {
				return m, tea.Batch(m.updateTOTPReal(), nextTickCmd)
			}
			// Return model with updated countdown and continue timer
			return m, nextTickCmd
		} else {
			// If we don't have a code or remaining time, generate one
			return m, tea.Batch(m.updateTOTPReal(), nextTickCmd)
		}
	}
	return m, nextTickCmd
```

## How to Test
1. Build the Go binary: `go build -o pwmgr-go ./cmd/pwmgr`
2. Start TUI: `./pwmgr-go tui` or just `./pwmgr-go`
3. Navigate to a TOTP entry (e.g., "Psono-totp", "test-totp")
4. Press Enter to view details
5. Press 's' to show secrets - the TOTP code should appear
6. Watch the progress bar - it should now countdown from 30 to 0 automatically
7. When it reaches 0, a new TOTP code should be generated

## Expected Behavior
- Progress bar shows remaining time and decrements automatically every second
- TOTP code refreshes when countdown reaches 0
- Progress bar visualization updates smoothly without requiring user interaction

## Verified Working
✅ CLI TOTP watch mode works correctly
✅ Go binary builds successfully
✅ TUI starts without errors
✅ TOTP entries are available for testing