#!/bin/bash
set -e

echo "Testing TOTP Auto-Refresh in TUI..."
echo "========================================="

# Verify we have TOTP entries to test with
echo "1. Checking for TOTP entries..."
./pwmgr-go totp-list

echo ""
echo "2. Verifying CLI TOTP countdown works..."
echo "Running TOTP watch mode for 8 seconds to verify basic functionality..."
timeout 8s ./pwmgr-go totp-code Psono-totp gerry --watch || echo "TOTP watch completed (expected timeout)"

echo ""
echo "3. Test Complete!"
echo ""
echo "Manual TUI Test Instructions:"
echo "=============================" 
echo "1. Run: ./pwmgr-go tui"
echo "2. Navigate to a TOTP entry (Psono-totp)"
echo "3. Press Enter to view details"
echo "4. Press 's' to show TOTP code"
echo "5. Watch the progress bar - it should countdown automatically"
echo "6. When it reaches 0, a new TOTP code should be generated"
echo ""
echo "Expected behavior:"
echo "- Progress bar counts down from 30 to 0 automatically (no user input needed)"
echo "- Visual progress bar updates every second"
echo "- TOTP code refreshes when countdown reaches 0"
echo ""
echo "If the progress bar is stuck or not counting down, the fix needs more work."