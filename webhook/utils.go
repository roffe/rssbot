package webhook

import (
	"log"
	"strconv"
	"strings"
)

// Hex2int converts a hex number to int
func Hex2int(hexStr string) int {
	// remove 0x suffix if found in the input string
	cleaned := strings.Replace(hexStr, "0x", "", -1)
	cleaned = strings.TrimPrefix(cleaned, "#")
	// base 16 for hexadecimal
	result, err := strconv.ParseInt(cleaned, 16, 64)
	if err != nil {
		log.Println(err)
		return 0
	}
	return int(result)
}
