#!/usr/bin/env bash

# Basic Test Suite for Password Manager
# Tests core functionality and integration

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Test configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
PWMGR="$PROJECT_DIR/bin/pwmgr"
PWGEN="$PROJECT_DIR/utils/pwgen-advanced"
BACKUP="$PROJECT_DIR/utils/pwmgr-backup"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Print functions
print_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
print_success() { echo -e "${GREEN}[PASS]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[WARN]${NC} $1"; }
print_error() { echo -e "${RED}[FAIL]${NC} $1" >&2; }

# Test result functions
test_pass() {
    TESTS_PASSED=$((TESTS_PASSED + 1))
    print_success "$1"
}

test_fail() {
    TESTS_FAILED=$((TESTS_FAILED + 1))
    print_error "$1"
}

run_test() {
    TESTS_RUN=$((TESTS_RUN + 1))
    local test_name="$1"
    local test_command="$2"
    
    print_info "Running test: $test_name"
    
    if eval "$test_command" >/dev/null 2>&1; then
        test_pass "$test_name"
        return 0
    else
        test_fail "$test_name"
        return 1
    fi
}

# Cleanup function
cleanup() {
    print_info "Cleaning up test data..."
    
    # Remove test passwords from keyring
    secret-tool clear service "test-service-1" 2>/dev/null || true
    secret-tool clear service "test-service-2" username "testuser" 2>/dev/null || true
    secret-tool clear service "test-github" 2>/dev/null || true
    
    # Remove test backup directory
    rm -rf "/tmp/pwmgr-test-backups" 2>/dev/null || true
    
    print_info "Cleanup completed"
}

# Trap cleanup on exit
trap cleanup EXIT

# Test script executability
test_executability() {
    print_info "Testing script executability..."
    
    run_test "Main script executable" "[[ -x '$PWMGR' ]]"
    run_test "Password generator executable" "[[ -x '$PWGEN' ]]"
    run_test "Backup utility executable" "[[ -x '$BACKUP' ]]"
}

# Test dependencies
test_dependencies() {
    print_info "Testing dependencies..."
    
    run_test "secret-tool available" "command -v secret-tool >/dev/null 2>&1"
    run_test "GNOME Keyring daemon running" "systemctl --user is-active gnome-keyring-daemon.service >/dev/null 2>&1"
    run_test "openssl available" "command -v openssl >/dev/null 2>&1"
    run_test "tar available" "command -v tar >/dev/null 2>&1"
}

# Test password manager initialization
test_initialization() {
    print_info "Testing password manager initialization..."
    
    run_test "Password manager initialization" "$PWMGR init"
    run_test "Configuration file created" "[[ -f '$PROJECT_DIR/config/pwmgr.conf' ]]"
    run_test "Config command works" "$PWMGR config"
}

# Test password operations
test_password_operations() {
    print_info "Testing password operations..."
    
    # Test adding passwords
    run_test "Add password without username" "echo 'testpassword123' | $PWMGR add test-service-1"
    run_test "Add password with username" "echo 'testpassword456' | $PWMGR add test-service-2 testuser"
    
    # Test retrieving passwords
    run_test "Get password without username" "[[ \$($PWMGR get test-service-1) == 'testpassword123' ]]"
    run_test "Get password with username" "[[ \$($PWMGR get test-service-2 testuser) == 'testpassword456' ]]"
    
    # Test listing passwords
    run_test "List passwords" "$PWMGR list | grep -q test-service"
    
    # Test updating passwords
    run_test "Update password" "echo 'newpassword123' | $PWMGR update test-service-1"
    run_test "Verify password updated" "[[ \$($PWMGR get test-service-1) == 'newpassword123' ]]"
    
    # Test searching passwords
    run_test "Search passwords" "$PWMGR search test | grep -q test-service"
    
    # Test deleting passwords
    run_test "Delete password" "echo 'y' | $PWMGR delete test-service-1"
    run_test "Verify password deleted" "! $PWMGR get test-service-1 >/dev/null 2>&1"
}

# Test password generation
test_password_generation() {
    print_info "Testing password generation..."
    
    run_test "Generate default password" "$PWMGR generate | grep -E '^.{16}$'"
    run_test "Generate 20-char password" "$PWMGR generate 20 | grep -E '^.{20}$'"
    
    # Advanced generator tests
    run_test "Advanced generator default" "$PWGEN | grep -E '^.{16}$'"
    run_test "Generate 5 passwords" "[[ \$($PWGEN -c 5 | wc -l) -eq 5 ]]"
    run_test "Generate alphanumeric password" "$PWGEN -t alphanum -l 12 | grep -E '^[A-Za-z0-9]{12}$'"
    run_test "Generate memorable password" "$PWGEN -t memorable -l 16 | grep -E '^.{16}$'"
    run_test "Generate passphrase" "$PWGEN -t passphrase | grep -E '^[A-Za-z-]+[0-9]+$'"
    run_test "Password strength check" "$PWGEN --check -l 16 | grep -q 'Strength:'"
}

# Test backup functionality
test_backup_functionality() {
    print_info "Testing backup functionality..."
    
    # Set up test backup directory
    local test_backup_dir="/tmp/pwmgr-test-backups"
    mkdir -p "$test_backup_dir"
    
    # Add a test password for backup
    echo 'backuptest123' | secret-tool store --label="Test password for backup" service "test-github" username "testuser"
    
    run_test "Create backup" "$BACKUP -d '$test_backup_dir' backup"
    run_test "List backups" "$BACKUP -d '$test_backup_dir' list | grep -q 'pwmgr-backup'"
    
    # Find the backup file
    local backup_file
    backup_file=$(find "$test_backup_dir" -name "pwmgr-backup-*.tar.gz" -type f | head -n1)
    
    if [[ -n "$backup_file" ]]; then
        run_test "Verify backup" "$BACKUP verify '$backup_file'"
        run_test "Clean backups" "$BACKUP -d '$test_backup_dir' clean"
    else
        test_fail "Backup file not found"
    fi
}

# Test error handling
test_error_handling() {
    print_info "Testing error handling..."
    
    run_test "Handle non-existent password" "! $PWMGR get non-existent-service >/dev/null 2>&1"
    run_test "Handle invalid command" "! $PWMGR invalid-command >/dev/null 2>&1"
    run_test "Handle missing service name" "! $PWMGR add >/dev/null 2>&1"
    run_test "Handle invalid password length" "! $PWMGR generate 999 >/dev/null 2>&1"
}

# Test help and version
test_help_and_version() {
    print_info "Testing help and version information..."
    
    run_test "Main script help" "$PWMGR --help | grep -q 'Password Manager'"
    run_test "Main script version" "$PWMGR --version | grep -q 'v1.0.0'"
    run_test "Advanced generator help" "$PWGEN --help | grep -q 'Advanced Password Generator'"
    run_test "Backup utility help" "$BACKUP --help | grep -q 'Backup & Restore'"
}

# Main test runner
main() {
    echo "========================================="
    echo "Password Manager - Basic Test Suite"
    echo "========================================="
    echo
    
    # Run test suites
    test_executability
    echo
    test_dependencies
    echo
    test_initialization
    echo
    test_password_operations
    echo
    test_password_generation
    echo
    test_backup_functionality
    echo
    test_error_handling
    echo
    test_help_and_version
    echo
    
    # Print summary
    echo "========================================="
    echo "Test Results Summary"
    echo "========================================="
    echo "Tests Run:    $TESTS_RUN"
    echo "Tests Passed: $TESTS_PASSED"
    echo "Tests Failed: $TESTS_FAILED"
    
    if [[ $TESTS_FAILED -eq 0 ]]; then
        print_success "All tests passed!"
        echo
        print_info "Your Password Manager is working correctly!"
        exit 0
    else
        print_error "$TESTS_FAILED test(s) failed!"
        echo
        print_warning "Please review the failed tests and check your setup."
        exit 1
    fi
}

# Run tests
main "$@"