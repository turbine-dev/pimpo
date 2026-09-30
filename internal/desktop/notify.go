// Package desktop shows system notifications on the computer Pimpo runs
// on, for the desktop app: approvals reach the owner even with the window
// closed.
package desktop

import (
	"context"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Run executes a command; tests replace it.
var Run = func(ctx context.Context, name string, args ...string) error {
	return RunEnv(ctx, nil, name, args...)
}

// RunEnv executes a command with extra environment variables on top of
// this process's own; tests replace it.
var RunEnv = func(ctx context.Context, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd.Run()
}

// Notify shows a notification with a title and a body.
func Notify(ctx context.Context, title, body string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	name, args, env := notifyCommand(runtime.GOOS, clean(title, 80), clean(body, 240))
	return RunEnv(ctx, env, name, args...)
}

// The Windows toast reads its text from the environment, so nothing the
// title or body holds ever becomes part of the PowerShell script.
const toastScript = "[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] > $null;" +
	"$t = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02);" +
	"$x = $t.GetElementsByTagName('text'); $x.Item(0).AppendChild($t.CreateTextNode($env:PIMPO_TITLE)) > $null; $x.Item(1).AppendChild($t.CreateTextNode($env:PIMPO_BODY)) > $null;" +
	"[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('Pimpo').Show([Windows.UI.Notifications.ToastNotification]::new($t))"

// notifyCommand builds the command that shows a notification. The text is
// always passed as data (arguments or environment), never inside a script.
func notifyCommand(goos, title, body string) (name string, args, env []string) {
	switch goos {
	case "darwin":
		return "osascript", []string{"-e", "on run argv", "-e", "display notification (item 2 of argv) with title (item 1 of argv)", "-e", "end run", "--", title, body}, nil
	case "windows":
		return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command", toastScript}, []string{"PIMPO_TITLE=" + title, "PIMPO_BODY=" + body}
	default:
		return "notify-send", []string{"--app-name=Pimpo", "--", title, body}, nil
	}
}

// Open shows a web link in the default browser of this computer.
func Open(ctx context.Context, link string) error {
	u, err := url.Parse(link)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "mailto") {
		return errors.New("only web and mail links open")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	switch runtime.GOOS {
	case "darwin":
		return Run(ctx, "open", u.String())
	case "windows":
		return Run(ctx, "rundll32", "url.dll,FileProtocolHandler", u.String())
	default:
		return Run(ctx, "xdg-open", u.String())
	}
}

func clean(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		s = string(r[:n-1]) + "…"
	}
	return s
}
