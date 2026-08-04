//go:build !windows

package password

import "os"

func openControllingTerminal() (terminalSession, error) {
	file, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &fileTerminal{input: file, output: file}, nil
}
