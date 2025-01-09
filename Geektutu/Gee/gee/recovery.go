package gee

import (
	"fmt"
	"log"
	"net/http"
	"runtime"
	"strings"
)

// print stack trace for debug.
func trace(message string) string {
	var pcs [32]uintptr
	// skip first 3 callers to simple the debug message.
	// callers[0] is Callers itself.
	// callers[1] is trace function.
	// callers[2] is defer func.
	n := runtime.Callers(3, pcs[:])

	var str strings.Builder
	str.WriteString(message + "\nTarceback:")
	for _, pc := range pcs[:n] {
		fn := runtime.FuncForPC(pc)
		file, line := fn.FileLine(pc)
		str.WriteString(fmt.Sprintf("\n\t%s:%d", file, line))
	}
	return str.String()
}

func Recovery() HandleFunc {
	return func(c *Context) {
		defer func() {
			if err := recover(); err != nil {
				message := fmt.Sprintf("%s", err)
				log.Printf("%s\n\n", trace(message))
				c.Fail(http.StatusInternalServerError, "Internal Server Error")
			}
		}()
		c.Next()
	}
}
