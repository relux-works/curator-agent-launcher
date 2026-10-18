package client

import (
	"fmt"
	"io"
)

// WriteDoorbell reports only validated session evidence, independently of a2a
// presence. Callers omit it in JSON output, which carries the complete record.
func WriteDoorbell(w io.Writer, session *Session) {
	if session == nil {
		return
	}
	d := session.Doorbell
	fmt.Fprintf(w, "curator-run: doorbell state=%s uncertain=%t", d.State, d.Uncertain)
	if d.Reason != nil {
		fmt.Fprintf(w, " reason=%s", *d.Reason)
	}
	fmt.Fprintln(w)
}
