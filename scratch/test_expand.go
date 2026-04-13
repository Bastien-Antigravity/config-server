package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	input := "${CF_IP:127.0.0.2}:${CF_PORT:3306}"
	expanded := os.Expand(input, func(s string) string {
		fmt.Printf("Expand callback for: %q\n", s)
		parts := strings.SplitN(s, ":", 2)
		val := os.Getenv(parts[0])
		if val == "" && len(parts) > 1 {
			val = parts[1]
		}
		return strings.Trim(val, "\"")
	})
	fmt.Printf("Result: %q\n", expanded)
}
