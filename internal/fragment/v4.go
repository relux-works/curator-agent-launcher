package fragment

// v4Policy validates the whole typed agreement, including presence. Older
// revisions never enter this path and reject these members as unknown.
func v4Policy(root Value, f *Fragment) error {
	if value, ok := root.Get("root_context"); ok {
		if f.Environment != EnvMuse {
			return invalid("/root_context", "only Muse admits root_context")
		}
		if err := closedObject("/root_context", value, []string{"exclusive"}, nil); err != nil {
			return err
		}
		exclusive, _ := value.Get("exclusive")
		if exclusive.Kind != KindBool || !exclusive.Bool {
			return invalid("/root_context/exclusive", "must equal true")
		}
		f.RootContext = &RootContext{Exclusive: true}
	}
	value, present := root.Get("credential_store")
	if !present {
		if f.Environment == EnvMuse {
			return invalid("/credential_store", "required for Muse")
		}
		return nil
	}
	if err := closedObject("/credential_store", value, []string{"selection"}, []string{"config_overrides"}); err != nil {
		return err
	}
	selection, err := enumMember("/credential_store", value, "selection", "native", "file", "keychain")
	if err != nil {
		return err
	}
	store := &CredentialStore{Selection: selection}
	overrides, hasOverrides := value.Get("config_overrides")
	expected := ""
	switch f.Environment {
	case EnvMuse:
		if selection != "file" {
			return invalid("/credential_store/selection", "Muse requires file")
		}
	case EnvCodexCLI:
		switch selection {
		case "file":
			expected = "file"
		case "keychain":
			expected = "keyring"
		}
	default:
		if selection != "native" {
			return invalid("/credential_store/selection", "adapter requires native")
		}
	}
	if expected == "" {
		if hasOverrides {
			return invalid("/credential_store/config_overrides", "overrides forbidden for this adapter/selection")
		}
	} else {
		if !hasOverrides {
			return invalid("/credential_store/config_overrides", "required for Codex file/keychain")
		}
		if err := closedObject("/credential_store/config_overrides", overrides, []string{"cli_auth_credentials_store"}, nil); err != nil {
			return err
		}
		actual, err := stringMember("/credential_store/config_overrides", overrides, "cli_auth_credentials_store")
		if err != nil {
			return err
		}
		if actual != expected {
			return invalid("/credential_store/config_overrides/cli_auth_credentials_store", "selection and override disagree")
		}
		store.ConfigOverrides = map[string]string{"cli_auth_credentials_store": expected}
	}
	f.CredentialStore = store
	return nil
}
