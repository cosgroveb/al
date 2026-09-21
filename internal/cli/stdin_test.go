package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"testing"
	"time"
)

type inheritedInput struct {
	*os.File
	ready *os.File
	once  sync.Once
}

func (r *inheritedInput) Read(data []byte) (int, error) {
	r.once.Do(func() {
		_, _ = r.ready.Write([]byte{1})
		_ = r.ready.Close()
	})
	return r.File.Read(data)
}

func TestStdinCancellationInheritedPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt cannot be sent to a process on Windows")
	}
	if os.Getenv("AL_TEST_STDIN_HELPER") == "1" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		input := &inheritedInput{File: os.Stdin, ready: os.NewFile(3, "ready")}
		status := Run(ctx, []string{"add", "--stdin", "--json"}, input, os.Stdout, os.Stderr)
		stop()
		os.Exit(status)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stdin, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = writer.Close() })
	ready, readyWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ready.Close(); _ = readyWriter.Close() })
	command := exec.Command(executable, "-test.run=^TestStdinCancellationInheritedPipe$")
	command.Env = append(os.Environ(), "AL_TEST_STDIN_HELPER=1")
	command.Stdin = stdin
	command.ExtraFiles = []*os.File{readyWriter}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	_ = readyWriter.Close()
	done := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = command.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		_ = command.Process.Kill()
		<-done
	})
	started := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(ready, make([]byte, 1))
		started <- err
	}()
	select {
	case err := <-started:
		if err != nil {
			t.Fatalf("stdin readiness: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("child process did not start reading stdin")
	}
	// Let the inherited descriptor enter its blocking read before sending SIGINT.
	time.Sleep(100 * time.Millisecond)
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled process remained blocked with stdin open")
	}
	if waitErr == nil || command.ProcessState.ExitCode() != 1 {
		t.Fatalf("exit = %d, wait error = %v", command.ProcessState.ExitCode(), waitErr)
	}
	result := decodeShellOutput(t, &stdout)
	if result.OK || result.Error == nil || result.Error.Code != "canceled" || stderr.Len() != 0 {
		t.Fatalf("cancellation: error %v, stderr %q", result.Error, stderr.String())
	}
}
