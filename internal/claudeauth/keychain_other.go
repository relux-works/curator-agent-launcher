//go:build !darwin

package claudeauth

import "context"

func readKeychain(context.Context, string, string) ([]byte, error) { return nil, ErrRefused }
