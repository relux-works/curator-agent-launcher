package hosted_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var testReceiver string
var testFDContract string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "hosted-receiver-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	testReceiver = filepath.Join(dir, "receiver")
	cmd := exec.Command("go", "build", "-o", testReceiver, "./testdata/receiver")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		fmt.Fprintln(os.Stderr, "helper build:", err)
		os.Exit(1)
	}
	testFDContract = filepath.Join(dir, "fdcontract")
	cmd = exec.Command("go", "build", "-o", testFDContract, "./testdata/fdcontract")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		os.RemoveAll(dir)
		fmt.Fprintln(os.Stderr, "helper build:", err)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
