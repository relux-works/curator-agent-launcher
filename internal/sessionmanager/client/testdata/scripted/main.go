// This test-only executable replays a finite script; it implements no manager
// storage, admission, provider or lifecycle semantics.
package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Step struct {
	Operation  string
	Request    json.RawMessage
	Body       string
	Exit       int
	Stderr     int
	Frames     []string
	RawFrame   []byte
	SleepMS    int
	Descendant bool
}
type Trace struct {
	PID, PGID, Child int
	Operation        string
	Args, Env        []string
	Request          []byte
	FDs              map[int]string
}

func must(err error) {
	if err != nil {
		fmt.Fprint(os.Stderr, "script setup failure")
		os.Exit(90)
	}
}
func main() {
	watchdog := time.AfterFunc(20*time.Second, func() { os.Exit(93) })
	defer watchdog.Stop()
	if len(os.Args) == 3 && os.Args[1] == "child" {
		for i := 0; i < 200; i++ {
			must(os.WriteFile(os.Args[2], []byte(strconv.Itoa(i)), 0600))
			time.Sleep(100 * time.Millisecond)
		}
		return
	}
	if len(os.Args) < 3 {
		os.Exit(90)
	}
	path, operation := os.Args[1], os.Args[2]
	pidLog, err := os.OpenFile(path+".pids", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	must(err)
	_, err = fmt.Fprintln(pidLog, os.Getpid())
	must(err)
	must(pidLog.Close())
	data, err := os.ReadFile(path)
	must(err)
	var steps []Step
	must(json.Unmarshal(data, &steps))
	index := 0
	if b, e := os.ReadFile(path + ".index"); e == nil {
		index, err = strconv.Atoi(string(b))
		must(err)
	}
	if index >= len(steps) || steps[index].Operation != operation {
		os.Exit(91)
	}
	step := steps[index]
	must(os.WriteFile(path+".index", []byte(strconv.Itoa(index+1)), 0600))
	input := os.Stdin
	if operation == "attach" {
		input = os.NewFile(3, "request")
	}
	request, err := io.ReadAll(io.LimitReader(input, 2<<20))
	must(err)
	if step.Request != nil && string(step.Request) != "null" {
		var got, want any
		must(json.Unmarshal(request, &got))
		must(json.Unmarshal(step.Request, &want))
		a, _ := json.Marshal(got)
		b, _ := json.Marshal(want)
		if string(a) != string(b) {
			os.Exit(92)
		}
	}
	pgid, _ := syscall.Getpgid(0)
	trace := Trace{PID: os.Getpid(), PGID: pgid, Operation: operation, Args: os.Args[2:], Env: os.Environ(), Request: request, FDs: map[int]string{}}
	for fd := 0; fd < 32; fd++ {
		var st syscall.Stat_t
		if syscall.Fstat(fd, &st) == nil {
			trace.FDs[fd] = fmt.Sprintf("%d:%d:%d", st.Dev, st.Ino, st.Mode)
		}
	}
	if step.Descendant {
		child := exec.Command(os.Args[0], "child", path+".child")
		must(child.Start())
		trace.Child = child.Process.Pid
		f, err := os.OpenFile(path+".pids", os.O_APPEND|os.O_WRONLY, 0600)
		must(err)
		_, err = fmt.Fprintln(f, trace.Child)
		must(err)
		must(f.Close())
		// This is bounded independently of the client under test. On success the
		// process group kills both; on a faulty transport both exit within twenty sec.
		defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	}
	traceData, _ := json.Marshal(trace)
	f, err := os.OpenFile(path+".trace", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	must(err)
	_, err = f.Write(append(traceData, '\n'))
	must(err)
	must(f.Close())
	if step.SleepMS > 0 {
		time.Sleep(time.Duration(step.SleepMS) * time.Millisecond)
	}
	if step.Stderr > 0 {
		_, err = io.CopyN(os.Stderr, strings.NewReader(strings.Repeat("sentinel-secret-stderr", (step.Stderr+21)/22+1)), int64(step.Stderr))
		must(err)
	}
	if operation == "attach" || operation == "launch-exact" {
		status := os.NewFile(4, "status")
		for _, body := range step.Frames {
			var h [4]byte
			binary.BigEndian.PutUint32(h[:], uint32(len(body)))
			_, err = status.Write(h[:])
			must(err)
			_, err = io.WriteString(status, body)
			must(err)
		}
		if step.RawFrame != nil {
			_, err = status.Write(step.RawFrame)
			must(err)
		}
	}
	_, err = io.WriteString(os.Stdout, step.Body)
	must(err)
	os.Exit(step.Exit)
}
