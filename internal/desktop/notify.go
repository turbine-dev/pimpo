// Package desktop shows system notifications on the computer Zodim runs
// on, for the desktop app: approvals reach the owner even with the window
// closed.
package desktop

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Run executes a command; tests replace it.
var Run = func(ctx context.Context, name string, args ...string) error {
	return exec.CommandContext(ctx, name, args...).Run()
}

// Notify shows a notification with a title and a body.
func Notify(ctx context.Context, title, body string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	title, body = clean(title, 80), clean(body, 240)
	switch runtime.GOOS {
	case "darwin":
		return Run(ctx, "osascript", "-e", "display notification "+quote(body)+" with title "+quote(title))
	case "windows":
		script := "[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null;" +
			"$t = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02);" +
			"$x = $t.GetElementsByTagName('text'); $x.Item(0).AppendChild($t.CreateTextNode(" + psQuote(title) + ")) > $null; $x.Item(1).AppendChild($t.CreateTextNode(" + psQuote(body) + ")) > $null;" +
			"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('Zodim').Show([Windows.UI.Notifications.ToastNotification]::new($t))"
		return Run(ctx, "powershell", "-NoProfile", "-Command", script)
	default:
		return Run(ctx, "notify-send", "--app-name=Zodim", title, body)
	}
}

func clean(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		s = string(r[:n-1]) + "…"
	}
	return s
}

// quote makes an AppleScript string literal; nothing in it can end the
// string or run code.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
