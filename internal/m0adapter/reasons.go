// Package m0adapter is the DRAFT M0 executor adapter: curator-run in the role
// of swarma-dispatcher spec/m0-executor-entry.md (DRAFT m0-executor-entry/1 at
// cf72502, the no-feeder amendment). It reads one closed, bounded entry from the immutable FD3, maps it
// to the exact 19-element run-codex argv and a one-shot stdin pipe, and
// execve-replaces itself with the pinned broker executable E.
//
// Every refusal happens before exec and exits ExitRefusal (78) with one
// sanitized reason line on stderr. Input values, prompt bytes and paths
// derived from input are never echoed. After a successful exec the process is
// the runner and its statuses are the runner's own.
//
// This is a proposal-stage artifact: the durable C, the qualified clock and the
// Claude runner variant are refusing seams (seams.go), so the installed binary
// refuses every launch until those mechanisms are published and pinned.
package m0adapter

import "syscall"

// ExitRefusal is the reserved adapter pre-exec refusal status (spec §5). It is
// disjoint from the runner-owned statuses 0, 1, 64, 69, 75, 124 and 130 and is
// emitted only before exec.
const ExitRefusal = 78

// diagnosticPrefix starts the single stderr line of a refusal.
const diagnosticPrefix = "curator-run-m0-adapter: refused "

// Reason is one closed snake_case refusal code. It names the failed rule,
// never a value.
type Reason string

// The closed reason-code set. reasonTable below must list every constant.
const (
	// Seams and installation literals.
	ReasonCUnpublished         Reason = "c_unpublished"
	ReasonPlatformUnsupported  Reason = "platform_unsupported"
	ReasonLiteralMissing       Reason = "literal_missing"
	ReasonLiteralInvalid       Reason = "literal_invalid"
	ReasonClockUnqualified     Reason = "clock_unqualified"
	ReasonRunnerVariantPending Reason = "runner_variant_unpinned"

	// Descriptor inventory, FD3 and the duplicate sweep.
	ReasonFDInventory       Reason = "fd_inventory_failed"
	ReasonFDInventoryLimit  Reason = "fd_inventory_limit"
	ReasonFDStandardMissing Reason = "fd_standard_missing"
	ReasonFDPlanMissing     Reason = "fd_plan_missing"
	ReasonFDPlanType        Reason = "fd_plan_type"
	ReasonFDPlanMode        Reason = "fd_plan_mode"
	ReasonFDPlanDuplicate   Reason = "fd_plan_duplicate"
	ReasonFDUnexpected      Reason = "fd_unexpected"
	ReasonFDPlanRead        Reason = "fd_plan_read"
	ReasonFDPlanUnstable    Reason = "fd_plan_unstable"
	ReasonFDClose           Reason = "fd_close_failed"
	ReasonFDSweep           Reason = "fd_sweep_failed"
	ReasonFDFinal           Reason = "fd_final_check"

	// Lexical and finite byte/work rules (spec §2).
	ReasonEntrySizeLimit      Reason = "entry_size_limit"
	ReasonEntryBOM            Reason = "entry_bom"
	ReasonEntryUTF8           Reason = "entry_utf8_invalid"
	ReasonEntrySurrogate      Reason = "entry_surrogate"
	ReasonEntrySyntax         Reason = "entry_syntax"
	ReasonEntryTrailing       Reason = "entry_trailing"
	ReasonEntryTokenLimit     Reason = "entry_token_limit"
	ReasonEntryDepthLimit     Reason = "entry_depth_limit"
	ReasonEntryArray          Reason = "entry_array_forbidden"
	ReasonEntryLiteral        Reason = "entry_literal_forbidden"
	ReasonEntryNoninteger     Reason = "entry_number_noninteger"
	ReasonEntryNegativeZero   Reason = "entry_number_negative_zero"
	ReasonEntryNumberRange    Reason = "entry_number_range"
	ReasonEntryNameLength     Reason = "entry_name_length"
	ReasonEntryStringTotal    Reason = "entry_string_total"
	ReasonEntryStringLength   Reason = "entry_string_length"
	ReasonEntryDuplicate      Reason = "entry_duplicate_member"
	ReasonEntryCanonicalLimit Reason = "entry_canonical_limit"
	ReasonEntryWorkerPlan     Reason = "entry_worker_plan_mismatch"
	ReasonEntryUnknown        Reason = "entry_member_unknown"
	ReasonEntryMissing        Reason = "entry_member_missing"
	ReasonEntryType           Reason = "entry_member_type"
	ReasonEntryConstant       Reason = "entry_constant_mismatch"
	ReasonEntryID             Reason = "entry_id_shape"
	ReasonEntryHash           Reason = "entry_hash_shape"
	ReasonEntrySerial         Reason = "entry_serial_range"
	ReasonEntryUID            Reason = "entry_uid_range"
	ReasonEntryKey            Reason = "entry_key_shape"
	ReasonEntryExecutionID    Reason = "entry_execution_id_shape"
	ReasonEntrySelector       Reason = "entry_selector_shape"
	ReasonEntryCatalogString  Reason = "entry_catalog_string_shape"
	ReasonEntryPromptLength   Reason = "entry_prompt_length"
	ReasonEntryPromptNUL      Reason = "entry_prompt_nul"
	ReasonEnvMalformed        Reason = "env_malformed"
	ReasonEnvUnexpected       Reason = "env_unexpected"
	ReasonEnvDuplicate        Reason = "env_duplicate"
	ReasonEnvMissing          Reason = "env_missing"
	ReasonEnvDenied           Reason = "env_denied_marker"
	ReasonEnvHomeMismatch     Reason = "env_home_mismatch"
	ReasonEnvTmpdir           Reason = "env_tmpdir_outside_home"
	ReasonPathLimit           Reason = "path_limit"
	ReasonClockRange          Reason = "clock_range"
	ReasonClockRegressed      Reason = "clock_regressed"
	ReasonTimeoutArithmetic   Reason = "timeout_arithmetic"
	ReasonTimeoutInsufficient Reason = "timeout_insufficient"
	ReasonArgvLimit           Reason = "argv_limit"
	ReasonArgvDenied          Reason = "argv_denied_marker"
	ReasonStdinPipe           Reason = "stdin_pipe_failed"
	ReasonStdinCapacity       Reason = "stdin_pipe_capacity"
	ReasonStdinWrite          Reason = "stdin_write_failed"
	ReasonExecFailed          Reason = "exec_failed"
	ReasonInternal            Reason = "internal_error"
)

// reasonTable is the one closed table of refusal codes. Exit writes only a
// code from this table; anything else is reported as internal_error.
var reasonTable = []Reason{
	ReasonCUnpublished, ReasonPlatformUnsupported, ReasonLiteralMissing,
	ReasonLiteralInvalid, ReasonClockUnqualified, ReasonRunnerVariantPending,
	ReasonFDInventory, ReasonFDInventoryLimit, ReasonFDStandardMissing,
	ReasonFDPlanMissing, ReasonFDPlanType, ReasonFDPlanMode,
	ReasonFDPlanDuplicate, ReasonFDUnexpected, ReasonFDPlanRead,
	ReasonFDPlanUnstable, ReasonFDClose, ReasonFDSweep, ReasonFDFinal,
	ReasonEntrySizeLimit, ReasonEntryBOM, ReasonEntryUTF8,
	ReasonEntrySurrogate, ReasonEntrySyntax, ReasonEntryTrailing,
	ReasonEntryTokenLimit, ReasonEntryDepthLimit, ReasonEntryArray,
	ReasonEntryLiteral, ReasonEntryNoninteger, ReasonEntryNegativeZero,
	ReasonEntryNumberRange, ReasonEntryNameLength, ReasonEntryStringTotal,
	ReasonEntryStringLength, ReasonEntryDuplicate, ReasonEntryCanonicalLimit,
	ReasonEntryWorkerPlan, ReasonEntryUnknown, ReasonEntryMissing,
	ReasonEntryType, ReasonEntryConstant, ReasonEntryID, ReasonEntryHash,
	ReasonEntrySerial, ReasonEntryUID, ReasonEntryKey,
	ReasonEntryExecutionID, ReasonEntrySelector, ReasonEntryCatalogString,
	ReasonEntryPromptLength, ReasonEntryPromptNUL, ReasonEnvMalformed,
	ReasonEnvUnexpected, ReasonEnvDuplicate, ReasonEnvMissing,
	ReasonEnvDenied, ReasonEnvHomeMismatch, ReasonEnvTmpdir, ReasonPathLimit,
	ReasonClockRange, ReasonClockRegressed, ReasonTimeoutArithmetic,
	ReasonTimeoutInsufficient, ReasonArgvLimit, ReasonArgvDenied,
	ReasonStdinPipe, ReasonStdinCapacity, ReasonStdinWrite, ReasonExecFailed,
	ReasonInternal,
}

func knownReason(r Reason) bool {
	for _, k := range reasonTable {
		if k == r {
			return true
		}
	}
	return false
}

// refusal is the only error type the adapter produces internally.
type refusal struct{ reason Reason }

func (r *refusal) Error() string { return string(r.reason) }

func refuse(r Reason) error { return &refusal{reason: r} }

// reasonOf maps an error to its closed reason. A foreign error is never
// relabelled as a specific rule.
func reasonOf(err error) Reason {
	if r, ok := err.(*refusal); ok && knownReason(r.reason) {
		return r.reason
	}
	return ReasonInternal
}

// diagnosticLine is the exact stderr line for a refusal.
func diagnosticLine(r Reason) []byte {
	if !knownReason(r) {
		r = ReasonInternal
	}
	return []byte(diagnosticPrefix + string(r) + "\n")
}

// Exit writes the one sanitized diagnostic line and exits 78. It is used only
// before exec; nothing else is written.
func Exit(r Reason) {
	line := diagnosticLine(r)
	for len(line) > 0 {
		n, err := syscall.Write(2, line)
		if err == syscall.EINTR {
			continue
		}
		if err != nil || n <= 0 {
			break
		}
		line = line[n:]
	}
	syscall.Exit(ExitRefusal)
}
