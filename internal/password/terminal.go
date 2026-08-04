package password

import "os"

type fileTerminal struct {
	input  *os.File
	output *os.File
}

func (t *fileTerminal) InputFD() int  { return int(t.input.Fd()) }
func (t *fileTerminal) OutputFD() int { return int(t.output.Fd()) }
func (t *fileTerminal) Write(data []byte) (int, error) {
	return t.output.Write(data)
}
func (t *fileTerminal) Close() error {
	if t.input == t.output {
		return t.input.Close()
	}
	inputErr := t.input.Close()
	outputErr := t.output.Close()
	if inputErr != nil {
		return inputErr
	}
	return outputErr
}
