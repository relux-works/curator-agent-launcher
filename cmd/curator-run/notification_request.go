package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/relux-works/curator-agent-launcher/internal/cli"
	"github.com/relux-works/curator-agent-launcher/internal/fragment"
	"github.com/relux-works/curator-agent-launcher/internal/launcherconfig"
	"github.com/relux-works/curator-agent-launcher/internal/werkstatt"
	schema "github.com/relux-works/curator-agent-launcher/schemas/session-manager"
)

// This is expected request data. Only the continuing manager's independently
// configured record source can admit it or create an attachment.
type notificationRequest struct {
	Contract           string                       `json:"contract"`
	Backend            string                       `json:"backend"`
	Workflow           notificationWorkflow         `json:"workflow"`
	Member             werkstatt.NotificationMember `json:"member"`
	AdmissionReference string                       `json:"admission_reference"`
	CallTimeoutMS      int                          `json:"call_timeout_ms"`
}
type notificationWorkflow struct {
	Reference string `json:"reference"`
	SHA256    string `json:"sha256"`
}

func notificationFromRecord(selected *launcherconfig.Notification, record *werkstatt.Record, frag *fragment.Fragment, cwd string, doorbell bool) (*notificationRequest, error) {
	if selected == nil || selected.Backend == "off" {
		return nil, nil
	}
	refuse := func() (*notificationRequest, error) {
		return nil, errors.New("selected Host notification requires a matching explicit member record without legacy doorbell")
	}
	if record == nil || record.Notification == nil || doorbell || record.Doorbell != nil || frag == nil ||
		record.Runtime != frag.Environment || record.Profile != frag.Profile.Name ||
		record.ProfileLockSHA256 == "" || record.ProfileLockSHA256 != frag.Profile.LockSHA256 {
		return refuse()
	}
	actual, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return refuse()
	}
	clone, err := filepath.EvalSymlinks(record.Clone)
	if err != nil || clone != actual {
		return refuse()
	}
	input := record.Notification
	n := &notificationRequest{Contract: selected.Contract, Backend: selected.Backend,
		Workflow: notificationWorkflow{Reference: selected.WorkflowReference, SHA256: selected.WorkflowSHA256},
		Member:   input.Member, AdmissionReference: input.AdmissionReference, CallTimeoutMS: input.CallTimeoutMS}
	data, err := json.Marshal(n)
	if err != nil || schema.Validate("notification-request.schema.json", data) != nil {
		return refuse()
	}
	return n, nil
}

// Plain selection reads only the existing record directory. It deliberately
// does not run the station service, select a model, copy child_env, or acquire
// station admission. Host retains responsibility for actual-source admission.
func plainNotification(ctx context.Context, inv cli.Invocation, frag *fragment.Fragment, deps launchDeps) (*notificationRequest, error) {
	selected := deps.selectedConfig.Notification
	if selected == nil || selected.Backend == "off" {
		if inv.MemberID != "" {
			return nil, errors.New("--member requires selected Host notification")
		}
		return nil, nil
	}
	if inv.MemberID == "" {
		return nil, errors.New("selected Host notification requires explicit --member")
	}
	record, _, err := (werkstatt.Loader{Directory: filepath.Join(deps.configResolver.Locations.User, "werkstatt")}).Load(ctx, inv.MemberID)
	if err != nil {
		return nil, err
	}
	cwd, err := deps.workdir()
	if err != nil {
		return nil, err
	}
	return notificationFromRecord(selected, record, frag, cwd, inv.A2A != "")
}
