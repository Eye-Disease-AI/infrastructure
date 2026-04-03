package log

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
)

func log(prefix string, format string, args ...any) {
	var result strings.Builder
	dateStr := time.Now().Format("02-01-2006 15:04:05")
	dateStr = color.HiBlackString(dateStr)
	_, _ = result.WriteString(dateStr)
	_, _ = result.WriteRune(' ')
	_, _ = result.WriteString(prefix)
	_, _ = result.WriteRune(' ')
	fmt.Fprintf(&result, format, args...)
	fmt.Println(result.String())
}

func Ok(format string, args ...any) {
	log(color.GreenString("[:)]"), format, args...)
}

func Wait(format string, args ...any) {
	log(color.CyanString("[..]"), format, args...)
}

func Err(format string, args ...any) {
	log(color.RedString("[:(]"), format, args...)
}

func Lookdown(format string, args ...any) {
	log(color.CyanString("[-v]"), format, args...)
}

func Lookup(format string, args ...any) {
	log(color.CyanString("[-^]"), format, args...)
}

func Info(format string, args ...any) {
	log(color.HiBlueString("[--]"), format, args...)
}
