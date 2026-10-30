//go:build !gatewayphases

// Package gatewayphase marks the points of a filtering-gateway launch where
// tests of the built launcher hold it to deliver a real signal. In a normal
// build Reach does nothing.
package gatewayphase

// Reach marks a named phase; it is a no-op unless the launcher is built with
// the gatewayphases tag.
func Reach(string) {}
