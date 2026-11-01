//go:build !darwin && !linux

package claudeauth

func readFile(string) ([]byte, error) { return nil, ErrRefused }
