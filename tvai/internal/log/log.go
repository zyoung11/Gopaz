package log

import (
	"fmt"
	"os"
)

const (
	cReset  = "\x1b[0m"
	cBlue   = "\x1b[38;2;138;173;244m"
	cGreen  = "\x1b[38;2;166;218;149m"
	cYellow = "\x1b[38;2;238;212;159m"
	cRed    = "\x1b[38;2;237;135;150m"
	cMauve  = "\x1b[38;2;198;160;246m"
	cYellowName = "\x1b[38;2;238;212;159m"
)

var colored = os.Getenv("NO_COLOR") == ""

func wrap(code, s string) string {
	if !colored {
		return s
	}
	return code + s + cReset
}

func Prompt(msg string)   { fmt.Println(wrap(cBlue, "[?] ") + msg) }
func Info(msg string)     { fmt.Println(wrap(cBlue, "[i] ") + msg) }
func Success(msg string)  { fmt.Println(wrap(cGreen, "[+] ") + msg) }
func Warn(msg string)     { fmt.Println(wrap(cYellow, "[!] ") + msg) }
func Error(msg string)    { fmt.Println(wrap(cRed, "[-] ") + msg) }

func Infof(format string, a ...any)    { Info(fmt.Sprintf(format, a...)) }
func Successf(format string, a ...any) { Success(fmt.Sprintf(format, a...)) }
func Warnf(format string, a ...any)    { Warn(fmt.Sprintf(format, a...)) }
func Errorf(format string, a ...any)   { Error(fmt.Sprintf(format, a...)) }

// Debugf 仅在 TVAI_DEBUG=1 时输出诊断信息。
func Debugf(format string, a ...any) {
	if os.Getenv("TVAI_DEBUG") == "1" {
		fmt.Println(wrap(cBlue, "[i] ") + fmt.Sprintf(format, a...))
	}
}

// Progress 渲染单行进度，用 \r 原地刷新。
func Progress(label string, cur, total int) {
	if total <= 0 {
		return
	}
	pct := float64(cur) / float64(total) * 100
	fmt.Printf("\r%s%s %d/%d (%.1f%%)%s", wrap(cBlue, "[i] "), label, cur, total, pct, cReset)
}

func ProgressDone() { fmt.Print("\r\x1b[K") }
