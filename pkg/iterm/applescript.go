package iterm

import (
	"fmt"
	"strings"
)

// ScriptIsRunning returns AppleScript to check if iTerm2 is running.
func ScriptIsRunning() string {
	return `tell application "System Events" to (name of processes) contains "iTerm2"`
}

// ScriptCreateWorktreeWindow returns AppleScript to create a new iTerm2 window
// with two panes: claude on top, shell on bottom.
func ScriptCreateWorktreeWindow(wtPath, sessionName string, noClaude bool) string {
	// Escape single quotes in paths for AppleScript
	safePath := escapeAppleScript(wtPath)
	safeName := escapeAppleScript(sessionName)

	claudeCmd := fmt.Sprintf("cd '%s' && claude", safePath)
	if noClaude {
		claudeCmd = fmt.Sprintf("cd '%s'", safePath)
	}

	return fmt.Sprintf(`tell application "iTerm2"
	set newWindow to (create window with default profile)
	tell newWindow
		tell current session of current tab
			set name to "%s:claude"
			write text "%s"
			set claudeID to unique ID
		end tell
		tell current session of current tab
			set shellSession to (split horizontally with default profile)
		end tell
		tell shellSession
			set name to "%s:shell"
			write text "cd '%s'"
			set shellID to unique ID
		end tell
	end tell
	return claudeID & "\t" & shellID
end tell`, safeName, claudeCmd, safeName, safePath)
}

// ScriptSessionExists returns AppleScript to check if a session ID exists.
func ScriptSessionExists(sessionID string) string {
	safe := escapeAppleScript(sessionID)
	return fmt.Sprintf(`tell application "iTerm2"
	repeat with w in windows
		repeat with t in tabs of w
			repeat with s in sessions of t
				if unique ID of s is "%s" then
					return "true"
				end if
			end repeat
		end repeat
	end repeat
	return "false"
end tell`, safe)
}

// ScriptFocusWindow returns AppleScript to focus the window containing a session.
func ScriptFocusWindow(sessionID string) string {
	safe := escapeAppleScript(sessionID)
	return fmt.Sprintf(`tell application "iTerm2"
	repeat with w in windows
		repeat with t in tabs of w
			repeat with s in sessions of t
				if unique ID of s is "%s" then
					select t
					tell w to select
					activate
					return true
				end if
			end repeat
		end repeat
	end repeat
	return false
end tell`, safe)
}

// ScriptCloseWindow returns AppleScript to close the window containing a session.
func ScriptCloseWindow(sessionID string) string {
	safe := escapeAppleScript(sessionID)
	return fmt.Sprintf(`tell application "iTerm2"
	repeat with w in windows
		repeat with t in tabs of w
			repeat with s in sessions of t
				if unique ID of s is "%s" then
					close w
					return true
				end if
			end repeat
		end repeat
	end repeat
	return false
end tell`, safe)
}

// escapeAppleScript escapes characters that could break AppleScript strings.
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
