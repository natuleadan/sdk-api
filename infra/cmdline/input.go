package cmdline

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// EnterToContinue let stdin waiting for an enter key to continue.
func EnterToContinue() {
	fmt.Print("Press 'Enter' to continue...")
	if _, err := bufio.NewReader(os.Stdin).ReadBytes('\n'); err != nil {
		fmt.Fprintf(os.Stderr, "cmdline: enter prompt read error: %v\n", err)
	}
}

// ReadLine shows prompt to stdout and read a line from stdin.
func ReadLine(prompt string) string {
	fmt.Print(prompt)
	input, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimSpace(input)
}
