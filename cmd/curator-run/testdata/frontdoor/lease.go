package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"syscall"
	"time"
)

func leaseCommand() {
	root := os.Getenv("HOME")
	lock, err := os.OpenFile(filepath.Join(root, "lease-fixture.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		os.Exit(99)
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX) != nil {
		os.Exit(99)
	}
	args := os.Args[1:]
	operation := args[2]
	group, _ := syscall.Getpgid(0)
	trace, _ := os.OpenFile(filepath.Join(root, "lease-trace.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if trace == nil {
		os.Exit(99)
	}
	_ = json.NewEncoder(trace).Encode(map[string]any{"operation": operation, "args": args, "pid": os.Getpid(), "group": group})
	trace.Close()
	ledgerPath := filepath.Join(root, "lease-ledger.json")
	records := []map[string]any{}
	raw, _ := os.ReadFile(ledgerPath)
	if len(raw) > 0 && json.Unmarshal(raw, &records) != nil {
		os.Exit(99)
	}
	result := func(reason string, value any) {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"schema_version": 1, "operation": operation, "ok": true, "exit": 0, "code": nil, "reason": reason, "result": value, "details": nil})
	}
	refuse := func(code, reason string) {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"schema_version": 1, "operation": operation, "ok": false, "exit": 1, "code": code, "reason": reason, "result": nil, "details": map[string]any{"lease_id": nil, "revision": nil, "holders": []any{}, "count": 0}})
	}
	if operation == "list" {
		result("listed", map[string]any{"leases": records})
		return
	}
	if operation != "acquire" {
		refuse("environment_lease_authority_refused", "authority_unavailable")
		os.Exit(1)
	}
	values := map[string]string{}
	for i := 3; i+1 < len(args); i += 2 {
		values[args[i]] = args[i+1]
	}
	var holder map[string]any
	if json.Unmarshal([]byte(values["--holder"]), &holder) != nil {
		os.Exit(98)
	}
	for _, r := range records {
		h, _ := r["holder"].(map[string]any)
		if h["request_id"] != values["--request-id"] {
			continue
		}
		if !reflect.DeepEqual(h, holder) || r["environment"] != values["--env"] || r["profile"] != values["--profile"] || r["lock_sha256"] != values["--lock-sha256"] {
			refuse("environment_lease_holder_mismatch", "request_conflict")
			os.Exit(1)
		}
		reason := "acquired"
		if r["phase"] == "released" {
			reason = "already_released"
		}
		if r["phase"] == "broken" {
			reason = "already_broken"
		}
		result(reason, map[string]any{"changed": false, "lease": r})
		return
	}
	r := map[string]any{"kind": "home-lease/1", "lease_id": "lease-" + values["--request-id"], "manager_root_id": "fixture-root", "home_id": values["--env"] + ":" + values["--profile"], "environment": values["--env"], "profile": values["--profile"], "lock_sha256": values["--lock-sha256"], "revision": 1, "phase": "prepared", "acquired_at": time.Now().UTC().Format(time.RFC3339Nano), "holder": holder, "observation": map[string]any{"state": "not_observed", "observed_at": nil}}
	records = append(records, r)
	raw, _ = json.Marshal(records)
	if os.WriteFile(ledgerPath, raw, 0600) != nil {
		os.Exit(99)
	}
	if mode, _ := os.ReadFile(filepath.Join(root, "lease-lost-reply")); len(mode) > 0 {
		os.Remove(filepath.Join(root, "lease-lost-reply"))
		os.Exit(1)
	}
	result("acquired", map[string]any{"changed": true, "lease": r})
}
