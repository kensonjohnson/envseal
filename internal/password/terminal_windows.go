//go:build windows

package password

import "os"

func openControllingTerminal() (terminalSession, error) {
	input, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	output, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0)
	if err != nil {
		_ = input.Close()
		return nil, err
	}
	return &fileTerminal{input: input, output: output}, nil
}
