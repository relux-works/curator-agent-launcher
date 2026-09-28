package main

import "testing"

func foreignOwnedConfigFile(*testing.T, string) (string, bool) { return "", false }

func currentUserIsRoot() bool { return false }
