package password

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestPromptSequences(t *testing.T) {
	tests := []struct {
		name      string
		responses [][]byte
		call      func(*Provider) ([]byte, []byte, error)
		wantOut   string
		wantFirst string
		wantNext  string
	}{
		{
			name:      "encrypt confirmation",
			responses: [][]byte{[]byte("new-secret"), []byte("new-secret")},
			call: func(provider *Provider) ([]byte, []byte, error) {
				password, err := provider.Encrypt()
				return password, nil, err
			},
			wantOut:   "Envseal password: \nConfirm Envseal password: \n",
			wantFirst: "new-secret",
		},
		{
			name:      "decrypt one prompt",
			responses: [][]byte{[]byte("existing-secret")},
			call: func(provider *Provider) ([]byte, []byte, error) {
				password, err := provider.Decrypt()
				return password, nil, err
			},
			wantOut:   "Envseal password: \n",
			wantFirst: "existing-secret",
		},
		{
			name:      "rotate current and replacement",
			responses: [][]byte{[]byte("old-secret"), []byte("new-secret"), []byte("new-secret")},
			call:      func(provider *Provider) ([]byte, []byte, error) { return provider.Rotate() },
			wantOut:   "Current Envseal password: \nNew Envseal password: \nConfirm new Envseal password: \n",
			wantFirst: "old-secret",
			wantNext:  "new-secret",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, terminal := newFakeProvider(test.responses, nil)
			first, next, err := test.call(provider)
			if err != nil {
				t.Fatalf("prompt error = %v", err)
			}
			defer Wipe(first)
			defer Wipe(next)
			if got := string(first); got != test.wantFirst {
				t.Fatalf("first password = %q, want %q", got, test.wantFirst)
			}
			if got := string(next); got != test.wantNext {
				t.Fatalf("next password = %q, want %q", got, test.wantNext)
			}
			if got := terminal.output.String(); got != test.wantOut {
				t.Fatalf("terminal output = %q, want %q", got, test.wantOut)
			}
			if !terminal.closed {
				t.Fatal("terminal was not closed")
			}
		})
	}
}

func TestPromptFailuresAreValueFree(t *testing.T) {
	secret := "do-not-include-this"
	tests := []struct {
		name      string
		responses [][]byte
		errs      []error
		terminal  *fakeTerminal
		want      error
	}{
		{name: "blank", responses: [][]byte{nil}, want: ErrBlankPassword},
		{name: "mismatch", responses: [][]byte{[]byte(secret), []byte("different")}, want: ErrPasswordMismatch},
		{name: "eof", errs: []error{io.EOF}, want: ErrPasswordCancelled},
		{name: "read failure", errs: []error{errors.New("device failure")}, want: ErrPasswordRead},
		{name: "non terminal", responses: [][]byte{[]byte(secret)}, terminal: &fakeTerminal{inputFD: 10, outputFD: 11}, want: ErrTerminalUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, terminal := newFakeProvider(test.responses, test.errs)
			if test.terminal != nil {
				terminal = test.terminal
				provider.open = func() (terminalSession, error) { return terminal, nil }
				provider.reader = &fakeReader{terminals: map[int]bool{}}
			}
			password, err := provider.Encrypt()
			if password != nil {
				t.Fatalf("password = %q, want nil", password)
			}
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if bytes.Contains([]byte(err.Error()), []byte(secret)) {
				t.Fatalf("error leaks password: %q", err)
			}
			if !terminal.closed {
				t.Fatal("terminal was not closed")
			}
		})
	}
}

func TestUnavailableTerminalDoesNotReadPassword(t *testing.T) {
	reader := &fakeReader{terminals: map[int]bool{}}
	terminal := &fakeTerminal{inputFD: 10, outputFD: 11}
	provider := &Provider{
		open:   func() (terminalSession, error) { return terminal, nil },
		reader: reader,
	}
	_, err := provider.Decrypt()
	if !errors.Is(err, ErrTerminalUnavailable) {
		t.Fatalf("Decrypt() error = %v, want terminal unavailable", err)
	}
	if reader.calls != 0 {
		t.Fatalf("ReadPassword calls = %d, want 0", reader.calls)
	}
}

type fakeTerminal struct {
	inputFD  int
	outputFD int
	output   bytes.Buffer
	closed   bool
}

func (t *fakeTerminal) InputFD() int                   { return t.inputFD }
func (t *fakeTerminal) OutputFD() int                  { return t.outputFD }
func (t *fakeTerminal) Write(data []byte) (int, error) { return t.output.Write(data) }
func (t *fakeTerminal) Close() error {
	t.closed = true
	return nil
}

type fakeReader struct {
	terminals map[int]bool
	responses [][]byte
	errs      []error
	calls     int
}

func (r *fakeReader) IsTerminal(fd int) bool { return r.terminals[fd] }
func (r *fakeReader) ReadPassword(int) ([]byte, error) {
	index := r.calls
	r.calls++
	if index < len(r.errs) && r.errs[index] != nil {
		return nil, r.errs[index]
	}
	if index >= len(r.responses) {
		return nil, io.EOF
	}
	return append([]byte(nil), r.responses[index]...), nil
}

func newFakeProvider(responses [][]byte, errs []error) (*Provider, *fakeTerminal) {
	terminal := &fakeTerminal{inputFD: 10, outputFD: 11}
	reader := &fakeReader{terminals: map[int]bool{10: true, 11: true}, responses: responses, errs: errs}
	provider := &Provider{
		open:   func() (terminalSession, error) { return terminal, nil },
		reader: reader,
	}
	return provider, terminal
}
