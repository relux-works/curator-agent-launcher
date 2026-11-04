package werkstatt

// NotificationMember contains explicit expected selectors. The continuing
// manager independently admits its actual configured member and Source facts.
type NotificationMember struct {
	MXID              string `json:"mxid"`
	AgentSelector     string `json:"agent_selector"`
	Connection        string `json:"connection"`
	ConfigurationRoot string `json:"configuration_root"`
}

// NotificationInput carries record-selected expected data only. Workflow is
// selected by Core. Neither this value nor its reference confers admission.
type NotificationInput struct {
	Member             NotificationMember `json:"member"`
	AdmissionReference string             `json:"admission_reference"`
	CallTimeoutMS      int                `json:"call_timeout_ms"`
}
