//go:build windows

package firewall

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
)

const managedRuleGroup = "GoDrop (StrangeFragment)"

// Sync создаёт или удаляет правило входящих подключений GoDrop.
// Правило разрешает только TCP-трафик из локальной подсети. Для изменения
// настроек брандмауэра Windows может запросить подтверждение администратора.
func Sync(enabled bool, port int) error {
	if enabled && (port < 1 || port > 65535) {
		return fmt.Errorf("некорректный порт правила брандмауэра: %d", port)
	}

	if err := runElevatedPowerShell(firewallScript(enabled, port)); err != nil {
		return fmt.Errorf(
			"не удалось изменить правило брандмауэра Windows — подтвердите запрос администратора и попробуйте снова: %w",
			err,
		)
	}

	return nil
}

func firewallScript(enabled bool, port int) string {
	if !enabled {
		return fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$group = '%s'
try {
    $rules = @(Get-NetFirewallRule -PolicyStore PersistentStore -Group $group -ErrorAction SilentlyContinue)
    if ($rules.Count -gt 0) {
        $rules | Remove-NetFirewallRule -ErrorAction Stop
    }
    exit 0
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
`, managedRuleGroup)
	}

	ruleName := fmt.Sprintf("GoDrop - LAN file transfer TCP %d", port)

	return fmt.Sprintf(`
$ErrorActionPreference = 'Stop'
$group = '%s'
$name = '%s'
$created = $false
try {
    # Remove a stale rule with this exact name, then create the replacement.
    $targetRules = @(Get-NetFirewallRule -PolicyStore PersistentStore -Group $group -DisplayName $name -ErrorAction SilentlyContinue)
    if ($targetRules.Count -gt 0) {
        $targetRules | Remove-NetFirewallRule -ErrorAction Stop
    }

    $ruleParams = @{
        PolicyStore = 'PersistentStore'
        DisplayName = $name
        Group = $group
        Direction = 'Inbound'
        Action = 'Allow'
        Enabled = $true
        Protocol = 'TCP'
        LocalPort = '%d'
        RemoteAddress = 'LocalSubnet'
        Profile = 'Any'
        Description = 'Managed by GoDrop.'
    }
    New-NetFirewallRule @ruleParams | Out-Null
    $created = $true

    # Keep only GoDrop's current port rule; unrelated firewall rules are untouched.
    $oldRules = @(Get-NetFirewallRule -PolicyStore PersistentStore -Group $group -ErrorAction SilentlyContinue | Where-Object { $_.DisplayName -ne $name })
    if ($oldRules.Count -gt 0) {
        $oldRules | Remove-NetFirewallRule -ErrorAction Stop
    }
    exit 0
} catch {
    $message = $_.Exception.Message
    if ($created) {
        $newRule = @(Get-NetFirewallRule -PolicyStore PersistentStore -Group $group -DisplayName $name -ErrorAction SilentlyContinue)
        if ($newRule.Count -gt 0) {
            $newRule | Remove-NetFirewallRule -ErrorAction SilentlyContinue
        }
    }
    [Console]::Error.WriteLine($message)
    exit 1
}
`, managedRuleGroup, ruleName, port)
}

func runElevatedPowerShell(script string) error {
	windowsDir := os.Getenv("SystemRoot")
	if windowsDir == "" {
		return errors.New("переменная SystemRoot не задана")
	}

	powershell := filepath.Join(
		windowsDir,
		"System32",
		"WindowsPowerShell",
		"v1.0",
		"powershell.exe",
	)
	if _, err := os.Stat(powershell); err != nil {
		return fmt.Errorf("не удалось найти Windows PowerShell: %w", err)
	}

	encodedScript := encodePowerShell(script)
	launcher := fmt.Sprintf(`
try {
    $arguments = '-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -WindowStyle Hidden -EncodedCommand %s'
    $process = Start-Process -FilePath (Join-Path $PSHOME 'powershell.exe') -ArgumentList $arguments -Verb RunAs -WindowStyle Hidden -Wait -PassThru -ErrorAction Stop
    exit $process.ExitCode
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
`, encodedScript)

	command := exec.Command(
		powershell,
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-WindowStyle",
		"Hidden",
		"-EncodedCommand",
		encodePowerShell(launcher),
	)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}

	if detail := strings.TrimSpace(string(output)); detail != "" {
		return errors.New(detail)
	}

	return err
}

// PowerShell's -EncodedCommand accepts UTF-16LE bytes encoded with Base64.
func encodePowerShell(script string) string {
	encoded := utf16.Encode([]rune(script))
	bytes := make([]byte, len(encoded)*2)

	for i, character := range encoded {
		bytes[i*2] = byte(character)
		bytes[i*2+1] = byte(character >> 8)
	}

	return base64.StdEncoding.EncodeToString(bytes)
}
