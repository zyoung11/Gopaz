package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	cReset      = "\x1b[0m"
	cBlue       = "\x1b[38;2;138;173;244m"
	cGreen      = "\x1b[38;2;166;218;149m"
	cYellow     = "\x1b[38;2;238;212;159m"
	cRed        = "\x1b[38;2;237;135;150m"
	cMauve      = "\x1b[38;2;198;160;246m"
	cYellowName = "\x1b[38;2;238;212;159m"
)

var colored = os.Getenv("NO_COLOR") == ""
var jsonEvents = os.Getenv("GOPAZ_PROGRESS") == "json"

func wrap(code, s string) string {
	if !colored {
		return s
	}
	return code + s + cReset
}

func emitLog(level, msg string) {
	if jsonEvents {
		b, _ := json.Marshal(map[string]any{"type": "log", "level": level, "msg": msg})
		fmt.Println(string(b))
		return
	}
	var code, prefix string
	switch level {
	case "success":
		code, prefix = cGreen, "[+] "
	case "warn":
		code, prefix = cYellow, "[!] "
	case "error":
		code, prefix = cRed, "[-] "
	case "prompt":
		code, prefix = cBlue, "[?] "
	default:
		code, prefix = cBlue, "[i] "
	}
	fmt.Println(wrap(code, prefix) + msg)
}

func Prompt(msg string)  { emitLog("prompt", msg) }
func Info(msg string)    { emitLog("info", msg) }
func Success(msg string) { emitLog("success", msg) }
func Warn(msg string)    { emitLog("warn", msg) }
func Error(msg string)   { emitLog("error", msg) }

func Infof(format string, a ...any)    { Info(fmt.Sprintf(format, a...)) }
func Successf(format string, a ...any) { Success(fmt.Sprintf(format, a...)) }
func Warnf(format string, a ...any)    { Warn(fmt.Sprintf(format, a...)) }
func Errorf(format string, a ...any)   { Error(fmt.Sprintf(format, a...)) }

// Debugf prints diagnostic output only when GOPAZ_DEBUG=1.
func Debugf(format string, a ...any) {
	if os.Getenv("GOPAZ_DEBUG") == "1" {
		emitLog("debug", fmt.Sprintf(format, a...))
	}
}

// Progress reports pipeline progress. In JSON mode it emits one event per call.
func Progress(label string, cur, total int) {
	if jsonEvents {
		pct := -1.0
		if total > 0 {
			pct = float64(cur) / float64(total) * 100
		}
		b, _ := json.Marshal(map[string]any{
			"type": "progress", "label": label, "cur": cur, "total": total, "pct": pct,
		})
		fmt.Println(string(b))
		return
	}
	if total <= 0 {
		return
	}
	pct := float64(cur) / float64(total) * 100
	fmt.Printf("\r%s%s %d/%d (%.1f%%)%s", wrap(cBlue, "[i] "), label, cur, total, pct, cReset)
}

// ProgressDone clears the progress line or emits a JSON completion event.
func ProgressDone() {
	if jsonEvents {
		fmt.Println(`{"type":"progressdone"}`)
		return
	}
	fmt.Print("\r\x1b[K")
}
